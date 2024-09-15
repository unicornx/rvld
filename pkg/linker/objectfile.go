package linker

import (
	"bytes"
	"debug/elf"
	"github.com/ksco/rvld/pkg/utils"
	"math"
)

/*
 * ObjectFile 是 InputFile 的子类
 * 除了继承 InputFile 的属性外，还具备以下属性
 *
 * @SymtabSec：一个指向符号表所对应的 ELF Section 的指针
 *
 * @SymtabShndxSec: 一个 32 位整数的数组
 *  背景知识，和 SHT_SYMTAB_SHNDX secion 有关
 *  符号表的每一项 Elf_Sym 中有个字段 st_shndx (符号所在 section 的 index)
 *  正常情况下，当一个符号定义在本 obj 文件中，则该值就是符号所在 section 的 index
 *  其他情况会有特殊值：譬如 SHN_ABS/SHN_UNDEF/... 具体参考 cxyxy
 *  但是 cxyxy 中没有提到一个 SHN_XINDEX， 如果是这个值，则说明当前 obj 文件的
 *  符号表 section 还对应一个 type 为 SHT_SYMTAB_SHNDX 的 section。这个 section 由
 *  一个 Elf32_Word 的数组组成，数组的个数和该 obj 文件的符号表的 entry 相同。即
 *  假设本 obj 文件涉及了 100 个符号，那么 SHT_SYMTAB_SHNDX section 的数组就有至多 100
 *  项，可能少于100，因为有些符号可能不是 local 的。
 *  主要是用于扩展，因为原来的 st_shndx 的类型是 Elf32_Half, 即 16 bit 宽，如果
 *  section 个数很多（超过 65535），则不够，需要用 32 位的扩展。
 *  也就是说当 section 个数很多，超出 Elf32_Half 能够表达的范围时，Elf_Sym 中
 *  的字段 st_shndx 就为 SHN_XINDEX，此时这个符号所在的 section 的 index 值我们就
 *  需要到一个特殊的 section，即 type 为 SHT_SYMTAB_SHNDX 的 section 中去查找。对应
 *  数组下标和符号表的下标一致。
 *
 * @Sections: 是一个 InputSection 的指针数组。
 *            与 obj 文件中 Elf section 一一对应的 InputSection，方便 linker 内部处理
 *            但需要注意，并不是所有的 ELF section 都会创建对应的 InputSection 对象
 *            所以说虽然 ObjectFile::Sections 数组的个数和 InputFile::ElfSections
 *            的个数相同，但 ObjectFile::Sections 中实际有效的 InputSection 的个数
 *            会小于 InputFile::ElfSections 的个数，另外注意到 ObjectFile::Sections
 *            数组成员存放的是 *InputSection，这也体现了如果某个 elf section 不需要
 *            创建对应的 InputSection，那么 ObjectFile::Sections[] 中对应的项只会
 *            占用一个指针的大小，不会浪费内存
 *
 * @MergeableSections：每个标记为 mergeable 的 InputSection 会转化为一个 MergeableSection，关系是一一对应
 *                     这个 MergeableSection 经过 split 处理后被分割
 *                     所有的 mergeable 的 InputSection 被转化后统一放在一个数组中，这个数组就是 MergeableSections 对象
 *                     
 *                     注意这个数组的个数和 ObjectFile::Sections 是一样的，
 *                     但是并不是每个 InputSection 都是 mergeable 的，所以 
 *                     ObjectFile::MergeableSections 中的有效数据小于 ObjectFile::Sections
 *                     的有效数据，所以这里 MergeableSections 数组的成员类型也是指针。
 */
type ObjectFile struct {
	/*
	 * SymtabSec 和 SymtabShndxSec 我理解还是 ELF 范畴的
	 * Sections 和 MergeableSections 是 linker 自身范畴的
	 * FIXME: InputFile 和 ObjectFile 的界限在哪里？感觉这里分得不是很
	 * 清楚。我的想法是 ELF 范畴的应该都划归到 InputFile 中去处理，Linker
	 * 范畴的划归 ObjectFile 中去处理。这里感觉有点乱。
	 */
	InputFile
	SymtabSec         *Shdr			// 由 ObjectFile::Parse 解析获取
	SymtabShndxSec    []uint32		// 由 ObjectFile::InitializeSections 解析获取
	Sections          []*InputSection	// 由 ObjectFile::InitializeSections 解析获取
	MergeableSections []*MergeableSection	// 由 ObjectFile::InitializeMergeableSections 解析获取
}

// 在 InputFile 基础上
// 仅仅多了初始化一个 IsAlive 成员
func NewObjectFile(file *File, isAlive bool) *ObjectFile {
	// 这个貌似是 go 的语法
	// NewInputFile() 这个函数，接受一个 *File 指针类型，这个函数会
	// 在堆上 new 一个 InputFile 类型的对象出来，
	// 然后赋值给 ObjectFile 的 InputFile 成员
	// 这样就完成了基类的创建。
	// 而 o 得到的是 ObjectFile 的地址，也就是说 o 是一个 ObjectFile 的指针类型
	o := &ObjectFile{InputFile: NewInputFile(file)}
	o.IsAlive = isAlive
	return o
}

// 进一步解析 object 文件
// 获取以下信息
// 符号表的相关信息，包括 
// ObjectFile::SymtabSec
// ObjectFile::FirstGlobal, 继承自 InputFile
// ObjectFile::ElfSyms，继承自 InputFile
// ObjectFile::SymbolStrtab，继承自 InputFile
//
// ObjectFile::Sections, 由 o.InitializeSections() 完成
//
// ObjectFile::Symbols， 由 o.InitializeSymbols() 完成
func (o *ObjectFile) Parse(ctx *Context) {
	// 获取并保存符号表 section header
	o.SymtabSec = o.FindSection(uint32(elf.SHT_SYMTAB))
	// 获取第一个 Global 符号的位置
	if o.SymtabSec != nil {
		o.FirstGlobal = int(o.SymtabSec.Info)
		// 将 ELF 文件中的符号表 entry 读出来存放在 ElfSyms 成员中
		o.FillUpElfSyms(o.SymtabSec)
		// 将 ELF 文件中存放符号字符串的 section 的 rawdata 导出存放用于后续分析
		o.SymbolStrtab = o.GetBytesFromIdx(int64(o.SymtabSec.Link))
	}

	// 根据对应 obj 文件中的 ELF section 初始化 ObjectFile::Sections
	o.InitializeSections(ctx)

	// 解析文件的符号，初始化 InputFile::Symbols。
	// LOCAL 符号放在 ObjectFile 中保存，GLOBAL 符号放在 Context 中保存
	o.InitializeSymbols(ctx)

	// 对 objectFile 中的 mergeable 的 section 进行处理
	// 处理后的结果存放在 ObjectFile::MergeableSections
	o.InitializeMergeableSections(ctx)
	
	// 跳过 ".eh_frame" 的 section
	o.SkipEhframeSections()
}

func (o *ObjectFile) InitializeSections(ctx *Context) {
	// obj 文件中的 InputSection 个数必然和 Section header 数组的 size 相同
	o.Sections = make([]*InputSection, len(o.ElfSections))
	// 将我们感兴趣的 section 转化为 InputSection 对象并存放在 Context::Sections
	// 数组中，不感兴趣的略过
	for i := 0; i < len(o.ElfSections); i++ {
		shdr := &o.ElfSections[i]
		switch elf.SectionType(shdr.Type) {
		case elf.SHT_GROUP, elf.SHT_SYMTAB, elf.SHT_STRTAB, elf.SHT_REL, elf.SHT_RELA,
			elf.SHT_NULL:
			break
		case elf.SHT_SYMTAB_SHNDX:
			o.FillUpSymtabShndxSec(shdr)
		default:
			// 剩下的都是我们感兴趣的 section，一一对应创建 InputSection
			// 如果深入 NewInputSection 函数，我们会发现
			// 在对所有 InputSection 都创建完后，Context::OutputSections
			// 也创建完毕，包含了所有需要输出的 section
			name := ElfGetName(o.InputFile.ShStrtab, shdr.Name)
			o.Sections[i] = NewInputSection(ctx, name, o, uint32(i))
		}
	}

	// FIXME 这段逻辑没有看明白
	// shdr 应该是某个 type 是 SHT_RELA 的 section，应该对应的是一个重定向表 section
	// RelsecIdx 难道不是应该就是这个 shdr 的属性吗？用于标识这个重定向表是对应于
	// 哪个 section
	// 为啥这里是 target := o.Sections[shdr.Info]
	for i := 0; i < len(o.ElfSections); i++ {
		shdr := &o.InputFile.ElfSections[i]
		if shdr.Type != uint32(elf.SHT_RELA) {
			continue
		}

		utils.Assert(shdr.Info < uint32(len(o.Sections)))
		if target := o.Sections[shdr.Info]; target != nil {
			utils.Assert(target.RelsecIdx == math.MaxUint32)
			target.RelsecIdx = uint32(i)
		}
	}
}

func (o *ObjectFile) FillUpSymtabShndxSec(s *Shdr) {
	bs := o.GetBytesFromShdr(s)
	o.SymtabShndxSec = utils.ReadSlice[uint32](bs, 4)
}

func (o *ObjectFile) InitializeSymbols(ctx *Context) {
	if o.SymtabSec == nil {
		return
	}

	// 创建 LocalSymbols 数组，LocalSymbols 数组的个数为 o.FirstGlobal
	o.LocalSymbols = make([]Symbol, o.FirstGlobal)
	// 并初始化为空值
	for i := 0; i < len(o.LocalSymbols); i++ {
		o.LocalSymbols[i] = *NewSymbol("")
	}
	// LocalSymbols 的 File 指向自身所在的 obj 文件
	o.LocalSymbols[0].File = o

	// 从 index 为 1 的符号开始，对 Local 符号对应的 Symbol 数组 LocalSymbols 进行初始化
	// 第一个（index==0）的符号无效的未定义符号，我们直接跳过
	for i := 1; i < len(o.LocalSymbols); i++ {
		esym := &o.ElfSyms[i]
		sym := &o.LocalSymbols[i]
		sym.Name = ElfGetName(o.SymbolStrtab, esym.Name)
		sym.File = o
		sym.Value = esym.Val //先填写为 Elf_Sym::st_value
		sym.SymIdx = i

		// 对于 !ABS 的 符号，设置其所在 section 的 index
		if !esym.IsAbs() {
			sym.SetInputSection(o.Sections[o.GetShndx(esym, i)])
		}
	}

	// 创建 ObjectFile::Symbols 并初始化
	o.Symbols = make([]*Symbol, len(o.ElfSyms))
	// ObjectFile::Symbols 由两部分组成，
	// 一部分是 LOCAL 符号，所以直接指向 ObjectFile::LocalSymbols 的成员
	for i := 0; i < len(o.LocalSymbols); i++ {
		o.Symbols[i] = &o.LocalSymbols[i]
	}
	// 另一部分是 GLOBAL 符号，这部分指向 Context::SymbolMap 的成员
	// 注意在 GetSymbolByName 的过程中会向 Context::SymbolMap 添加。所以对所有
	// obj 文件执行完一遍 Parse 后，Context::SymbolMap 中会包含所有的 GLOBAL 符号
	for i := len(o.LocalSymbols); i < len(o.ElfSyms); i++ {
		esym := &o.ElfSyms[i]
		name := ElfGetName(o.SymbolStrtab, esym.Name)
		o.Symbols[i] = GetSymbolByName(ctx, name)
	}
}

func (o *ObjectFile) GetShndx(esym *Sym, idx int) int64 {
	utils.Assert(idx >= 0 && idx < len(o.ElfSyms))

	if esym.Shndx == uint16(elf.SHN_XINDEX) {
		return int64(o.SymtabShndxSec[idx])
	}
	return int64(esym.Shndx)
}

/*
```c
static void foo()
{
        printf("local");
}

int main()
{
        printf("hello");
        return 0;
}
```

Symbol table '.symtab' contains 7 entries:
   Num:    Value          Size Type    Bind   Vis      Ndx Name
     0: 0000000000000000     0 NOTYPE  LOCAL  DEFAULT  UND 
     1: 0000000000000000     0 FILE    LOCAL  DEFAULT  ABS hello.c
     2: 0000000000000000     0 SECTION LOCAL  DEFAULT    1 .text
     3: 0000000000000000     0 SECTION LOCAL  DEFAULT    5 .rodata
     4: 0000000000000000    31 FUNC    LOCAL  DEFAULT    1 foo
     5: 0000000000000000     0 NOTYPE  GLOBAL DEFAULT  UND printf
     6: 000000000000001f    35 FUNC    GLOBAL DEFAULT    1 main
*/

// 本质上就是找本module定义的 GLOBAL 符号
func (o *ObjectFile) ResolveSymbols() {
	// LOCAL 符号不需要 resolve，只处理 GLOBAL 符号
	for i := o.FirstGlobal; i < len(o.ElfSyms); i++ {
		sym := o.Symbols[i]
		esym := &o.ElfSyms[i]

		// 如果这个符号不是自身 obj 定义的，则略过
		if esym.IsUndef() {
			continue
		}

		var isec *InputSection
		// 对于不是 ABS 的符号，尝试获取该符号所在的 section
		// 此时该符号差不多就是本地定义的 GLOBAL 符号了
		if !esym.IsAbs() {
			isec = o.GetSection(esym, i)
			if isec == nil {
				continue
			}
		}

		// 如果这个本地的符号还没有 resolve，则 resolve
		if sym.File == nil {
			sym.File = o
			sym.SetInputSection(isec)
			sym.Value = esym.Val
			sym.SymIdx = i
		}
	}
}

func (o *ObjectFile) GetSection(esym *Sym, idx int) *InputSection {
	return o.Sections[o.GetShndx(esym, idx)]
}

// 判断一个 obj 中是否存在 UDEF 的 GLOBAL 符号引用
// 如果存在并且定义这个符号的外部 obj 没有被标识为 alive 则标识之，同时将这个 obj
// 文件也加入 roots（通过 feeder 这个回调函数）
func (o *ObjectFile) MarkLiveObjects(feeder func(*ObjectFile)) {
	utils.Assert(o.IsAlive)

	for i := o.FirstGlobal; i < len(o.ElfSyms); i++ {
		sym := o.Symbols[i]
		esym := &o.ElfSyms[i]

		// FIXME：没有看懂，感觉此类符号就直接跳过了
		// UNDEF 的 GLOBAL 符号难道此时的 File 成员不为 nil?
		if sym.File == nil {
			continue
		}

		// 如果某个符号是 UNDEF，说明这个符号定义在外部 obj 中
		// 则我们需要将这个外部的 obj 文件也标记为 Alive
		if esym.IsUndef() && !sym.File.IsAlive {
			sym.File.IsAlive = true
			feeder(sym.File)
		}
	}
}

// 针对一个 obj 文件中的所有 GLOBAL 符号
// 如果这个符号引用所对应的符号定义在本地 module 中的，则执行 Symbol::Clear
//
// 总而言之，我们的目的是清理掉那些我们不关心的 obj 文件定义的符号就对了
// 感觉这个函数应该定义在 Context 中，go 语言这里很让人迷惑，实际上是操作的 Symbol 指针，而指针在 Context 的 SymbolMap 中
func (o *ObjectFile) ClearSymbols() {
	for _, sym := range o.Symbols[o.FirstGlobal:] {
		if sym.File == o {
			sym.Clear()
		}
	}
}

// 对一个 ObjectFile 中的所有的 InputSection 遍历处理
// 如果这个 InputSection 是 mergeable 的，且还未 split 过，则调用 splitSection
// 将这个 InputSection 的 rawdata 进行 split 处理
// 处理后的结果是一个 MergeableSections 的对象，并存放在 ObjectFile::MergeableSections
// 中待后续进一步处理
//
// 具体的处理由 splitSection 完成，也就是将 section中的元素分割开，便于后继处理
// 注意分割处理后 isec.IsAlive 就从 true 变为 false
// FIXME：所以我现在理解这里之所以 
func (o *ObjectFile) InitializeMergeableSections(ctx *Context) {
	o.MergeableSections = make([]*MergeableSection, len(o.Sections))
	for i := 0; i < len(o.Sections); i++ {
		isec := o.Sections[i]
		if isec != nil && isec.IsAlive &&
			isec.Shdr().Flags&uint64(elf.SHF_MERGE) != 0 {
			o.MergeableSections[i] = splitSection(ctx, isec)
			isec.IsAlive = false
		}
	}
}

func findNull(data []byte, entSize int) int {
	if entSize == 1 {
		return bytes.Index(data, []byte{0})
	}

	for i := 0; i <= len(data)-entSize; i += entSize {
		bs := data[i : i+entSize]
		if utils.AllZeros(bs) {
			return i
		}
	}

	return -1
}

// 这个函数的作用是：
// 首先对输入的具备 SHF_MERGE 的 InputSection 对应创建一个 MergeableSection
// 然后将的 section 的 rawdata 分割成元素，存放在返回的
// MergeableSection 对象中
// 值得注意的是，这里分割的结果，只是把分割后的元素存放在
// MergeableSection::Strs（存放分割后的内容）
// 和
// MergeableSection::FragOffsets（存放分割后的偏移量）
// 并没有放到 MergeableSection::Fragments 中去
// 最终放到 MergeableSection::Fragments 中的动作是由 RegisterSectionPieces 完成。Why FIXME
//
// 另外，注意根据 Mergable Section 的概念, 其中元素分为两种类型，所以 split 的方式也分为两种
func splitSection(ctx *Context, isec *InputSection) *MergeableSection {
	m := &MergeableSection{}
	shdr := isec.Shdr()

	// 从 Context 中获取 merged section
	// 每个 name/flags/types 三元组相同的 MergeableSection 最终都会被 merge 
	// 到一个 MergedSection 中去
	// 所以这里就是在找对应的 MergedSection
	m.Parent = GetMergedSectionInstance(ctx, isec.Name(), shdr.Type,
		shdr.Flags)
	m.P2Align = isec.P2Align

	// 下面开始 split
	data := isec.Contents
	offset := uint64(0)
	if shdr.Flags&uint64(elf.SHF_STRINGS) != 0 {
		// 该 mergeable section 中的元素类型为 string 类型，元素之间以 '\0' 分割
		// 
		for len(data) > 0 {
			end := findNull(data, int(shdr.EntSize))
			if end == -1 {
				utils.Fatal("string is not null terminated")
			}

			sz := uint64(end) + shdr.EntSize
			substr := data[:sz]
			data = data[sz:]
			m.Strs = append(m.Strs, string(substr))
			m.FragOffsets = append(m.FragOffsets, uint32(offset))
			offset += sz
		}
	} else {
		// 元素的第二种类型是固定长度的 data
		if uint64(len(data))%shdr.EntSize != 0 {
			utils.Fatal("section size is not multiple of entsize")
		}

		for len(data) > 0 {
			substr := data[:shdr.EntSize]
			data = data[shdr.EntSize:]
			m.Strs = append(m.Strs, string(substr))
			m.FragOffsets = append(m.FragOffsets, uint32(offset))
			offset += shdr.EntSize
		}
	}

	return m
}

func (o *ObjectFile) RegisterSectionPieces() {
	for _, m := range o.MergeableSections {
		if m == nil {
			continue
		}

		m.Fragments = make([]*SectionFragment, 0, len(m.Strs))
		for i := 0; i < len(m.Strs); i++ {
			m.Fragments = append(m.Fragments,
				m.Parent.Insert(m.Strs[i], uint32(m.P2Align)))
		}
	}

	for i := 1; i < len(o.ElfSyms); i++ {
		sym := o.Symbols[i]
		esym := &o.ElfSyms[i]

		if esym.IsAbs() || esym.IsUndef() || esym.IsCommon() {
			continue
		}

		m := o.MergeableSections[o.GetShndx(esym, i)]
		if m == nil {
			continue
		}

		frag, fragOffset := m.GetFragment(uint32(esym.Val))
		if frag == nil {
			utils.Fatal("bad symbol value")
		}
		sym.SetSectionFragment(frag)
		sym.Value = uint64(fragOffset)
	}
}

func (o *ObjectFile) SkipEhframeSections() {
	for _, isec := range o.Sections {
		if isec != nil && isec.IsAlive && isec.Name() == ".eh_frame" {
			isec.IsAlive = false
		}
	}
}

func (o *ObjectFile) ScanRelocations() {
	for _, isec := range o.Sections {
		if isec != nil && isec.IsAlive &&
			isec.Shdr().Flags&uint64(elf.SHF_ALLOC) != 0 {
			isec.ScanRelocations()
		}
	}
}
