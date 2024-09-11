package linker

import (
	"debug/elf"
	"github.com/ksco/rvld/pkg/utils"
	"math"
	"sort"
)

func ResolveSymbols(ctx *Context) {
	// 遍历上下文中的 Objs，也就是命令行里输入的所有 obj 文件，
	// 对每个 obj 文件调用 ObjectFile::ResolveSymbols
	// 这里是要 resolve 所有 Bind == GLOBAL 的符号
	// 本质上就是要找到定义这些符号的 module
	// 也就是遍历所有的 module，看这些 module 自己定义了哪些 GLOBAL 符号
	// 参考 ObjectFile::ResolveSymbols()
	// 注意更新的结果体现在 Context::SymbolMap 中
	for _, file := range ctx.Objs {
		file.ResolveSymbols()
	}

	// 这里是调用 passes::MarkLiveObjects()
	// 将所有存在未决 GLOBAL 符号的 obj 全部标识出来，标识的方法是 InputFile::IsAlive 为 true
	// FIXME：我觉得 MarkLiveObjects 这一步其实已经不算是 ResloveSymbols 了
	// 下面三步可以提出来作为一个单独的 Mark Live
	MarkLiveObjects(ctx)

	// 遍历所有输入的 obj 文件，如果这个文件不需要参与解析的，则做一遍清理
	// 清理，后面这些 not alive 的 objfile 对象都会被从 Context::Objs 中移除了。
	// 具体看 ObjectFile::ClearSymbols()
	// 实际上这里的目的是把 non-alive 的也就是我们不关心的 obj 文件定义的
	// 符号清空，最终影响的是 Context::SymbolMap
	// FIXME 这里的代码容易让人迷惑。最好做成 Context 的行为。
	for _, file := range ctx.Objs {
		if !file.IsAlive {
			file.ClearSymbols()
		}
	}

	// RemoveIf 定义在 utils.go，就是缩减数组
	ctx.Objs = utils.RemoveIf[*ObjectFile](ctx.Objs, func(file *ObjectFile) bool {
		return !file.IsAlive
	})
}


func MarkLiveObjects(ctx *Context) {
	// 先创建一个空数组 roots，数组的成员是 ObjectFile 的指针类型
	// 这里的 roots 实现上类似一个队列，先进先出
	// 我们会先将默认标记为 alive 的 obj 入队
	// 然后对队列中的 obj 依次执行 MarkLiveObjects，处理完后出队
	// 注意在对 obj 文件执行 MarkLiveObjects 的过程中，可能会发现被依赖的新的
	// obj 文件，则会继续入队。
	// 这么做的结果就是，会像一个链条一样，将所有依赖的 obj 文件都查找并标识
	// 出来。
	// 举个例子：假设我们编写的项目有1个文件 a.o
	// a.o 中引用了外部符号 s1，s2。
	// libX.a 中有 1.o (定义了 s1), 2.o（定义了 s2，引用外部 s3） 和 3.o（定义了 s3），4.o(定义了 s4)
	// a.o (s1  s2)
	//      |    |
	//      V    V
	//      1.o  2.o (s3)
	//                 |
	//                 V
	//                 3.o
	// roots 的变化规律如下：
	// roots = {a.o}
	// roots = {1.o, 2.o} // 处理完 a.o 被移除
	// roots = {2.o} // 处理完 1.o 被移除
	// roots = {3.o} // 处理完 2.o 被移除
	// roots = {} // 处理完 3.o 被移除
	roots := make([]*ObjectFile, 0)
	
	// 初始化 roots
	// 遍历上下文中的 Objs，如果这个文件被标记为 Alive 则说明需要执行符号解析
	// 即这个 obj 文件中存在未解析的 GLOBAL 符号引用
	// 将该文件加入 roots 数组中等待后继继续处理
	for _, file := range ctx.Objs {
		if file.IsAlive {
			roots = append(roots, file)
		}
	}

	utils.Assert(len(roots) > 0)

	// 此时对 roots 数组进行遍历，
	for len(roots) > 0 {
		// 取出 roots 数组的第一个元素
		file := roots[0]
		
		// 如果不是 Alive 则 contine
		// FIXME ，这个是不是有问题？如果真的出现，continue 后又
		// 继续取第一个，roots 长度也不变，导致进入死循环
		// 这个条件不可能满足的吧，前面能加入 roots 的必然满足 alive
		// 所以这个判断感觉是多此一举
		if !file.IsAlive {
			continue
		}

		// 走到这里一定是 alive 的。则调用 ObjectFile::MarkLiveObjects
		// 传入的参数是一个函数，这个函数是：
		// func(file *ObjectFile) {
		//	roots = append(roots, file)
		// }
		// 我现在的理解是会将被别的 obj file 依赖的 obj file 也加入 roots。
		// 这些 obj 主要指来自 archive 文件中的 obj。
		file.MarkLiveObjects(func(file *ObjectFile) {
			roots = append(roots, file)
		})

		// 当前队列中的第一个 obj 文件处理完毕
		// 出队
		// roots 数组缩小一个，roots 的第一个元素被移除
		roots = roots[1:]
	}
}

func RegisterSectionPieces(ctx *Context) {
	for _, file := range ctx.Objs {
		file.RegisterSectionPieces()
	}
}

func CreateSyntheticSections(ctx *Context) {
	push := func(chunk Chunker) Chunker {
		ctx.Chunks = append(ctx.Chunks, chunk)
		return chunk
	}

	ctx.Ehdr = push(NewOutputEhdr()).(*OutputEhdr)
	ctx.Phdr = push(NewOutputPhdr()).(*OutputPhdr)
	ctx.Shdr = push(NewOutputShdr()).(*OutputShdr)
	ctx.Got = push(NewGotSection()).(*GotSection)
}

func SetOutputSectionOffsets(ctx *Context) uint64 {
	addr := IMAGE_BASE
	for _, chunk := range ctx.Chunks {
		if chunk.GetShdr().Flags&uint64(elf.SHF_ALLOC) == 0 {
			continue
		}

		addr = utils.AlignTo(addr, chunk.GetShdr().AddrAlign)
		chunk.GetShdr().Addr = addr

		if !isTbss(chunk) {
			addr += chunk.GetShdr().Size
		}
	}

	i := 0
	first := ctx.Chunks[0]
	for {
		shdr := ctx.Chunks[i].GetShdr()
		shdr.Offset = shdr.Addr - first.GetShdr().Addr
		i++

		if i >= len(ctx.Chunks) ||
			ctx.Chunks[i].GetShdr().Flags&uint64(elf.SHF_ALLOC) == 0 {
			break
		}
	}

	lastShdr := ctx.Chunks[i-1].GetShdr()
	fileoff := lastShdr.Offset + lastShdr.Size

	for ; i < len(ctx.Chunks); i++ {
		shdr := ctx.Chunks[i].GetShdr()
		fileoff = utils.AlignTo(fileoff, shdr.AddrAlign)
		shdr.Offset = fileoff
		fileoff += shdr.Size
	}

	ctx.Phdr.UpdateShdr(ctx)
	return fileoff
}

func BinSections(ctx *Context) {
	group := make([][]*InputSection, len(ctx.OutputSections))
	for _, file := range ctx.Objs {
		for _, isec := range file.Sections {
			if isec == nil || !isec.IsAlive {
				continue
			}

			idx := isec.OutputSection.Idx
			group[idx] = append(group[idx], isec)
		}
	}

	for idx, osec := range ctx.OutputSections {
		osec.Members = group[idx]
	}
}

func CollectOutputSections(ctx *Context) []Chunker {
	osecs := make([]Chunker, 0)
	for _, osec := range ctx.OutputSections {
		if len(osec.Members) > 0 {
			osecs = append(osecs, osec)
		}
	}

	for _, osec := range ctx.MergedSections {
		if osec.Shdr.Size > 0 {
			osecs = append(osecs, osec)
		}
	}

	return osecs
}

func ComputeSectionSizes(ctx *Context) {
	for _, osec := range ctx.OutputSections {
		offset := uint64(0)
		p2align := int64(0)

		for _, isec := range osec.Members {
			offset = utils.AlignTo(offset, 1<<isec.P2Align)
			isec.Offset = uint32(offset)
			offset += uint64(isec.ShSize)
			p2align = int64(math.Max(float64(p2align), float64(isec.P2Align)))
		}

		osec.Shdr.Size = offset
		osec.Shdr.AddrAlign = 1 << p2align
	}
}

func SortOutputSections(ctx *Context) {
	rank := func(chunk Chunker) int32 {
		typ := chunk.GetShdr().Type
		flags := chunk.GetShdr().Flags

		if flags&uint64(elf.SHF_ALLOC) == 0 {
			return math.MaxInt32 - 1
		}
		if chunk == ctx.Shdr {
			return math.MaxInt32
		}
		if chunk == ctx.Ehdr {
			return 0
		}
		if chunk == ctx.Phdr {
			return 1
		}
		if typ == uint32(elf.SHT_NOTE) {
			return 2
		}

		b2i := func(b bool) int {
			if b {
				return 1
			}
			return 0
		}

		writeable := b2i(flags&uint64(elf.SHF_WRITE) != 0)
		notExec := b2i(flags&uint64(elf.SHF_EXECINSTR) == 0)
		notTls := b2i(flags&uint64(elf.SHF_TLS) == 0)
		isBss := b2i(typ == uint32(elf.SHT_NOBITS))

		return int32(writeable<<7 | notExec<<6 | notTls<<5 | isBss<<4)
	}

	sort.SliceStable(ctx.Chunks, func(i, j int) bool {
		return rank(ctx.Chunks[i]) < rank(ctx.Chunks[j])
	})
}

func ComputeMergedSectionSizes(ctx *Context) {
	for _, osec := range ctx.MergedSections {
		osec.AssignOffsets()
	}
}

func ScanRelocations(ctx *Context) {
	for _, file := range ctx.Objs {
		file.ScanRelocations()
	}

	syms := make([]*Symbol, 0)
	for _, file := range ctx.Objs {
		for _, sym := range file.Symbols {
			if sym.File == file && sym.Flags != 0 {
				syms = append(syms, sym)
			}
		}
	}

	for _, sym := range syms {
		if sym.Flags&NeedsGotTp != 0 {
			ctx.Got.AddGotTpSymbol(sym)
		}

		sym.Flags = 0
	}
}

func isTbss(chunk Chunker) bool {
	shdr := chunk.GetShdr()
	return shdr.Type == uint32(elf.SHT_NOBITS) &&
		shdr.Flags&uint64(elf.SHF_TLS) != 0
}
