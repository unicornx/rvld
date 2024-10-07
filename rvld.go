package main

import (
	"fmt"
	"github.com/ksco/rvld/pkg/linker"
	"github.com/ksco/rvld/pkg/utils"
	"os"
	"path/filepath"
	"strings"
)

var version string

func main() {
	ctx := linker.NewContext()
	// 解析命令行选项和参数
	remaining := parseArgs(ctx)

	// 如果命令行中没有明确指明 "-m target", 那么我们自己去看一下 .o 文件的类型
	// 目前只会根据第一个遇到的可识别 obj 文件的 ARCH 类型作为 machine type
	if ctx.Args.Emulation == linker.MachineTypeNone {
		for _, filename := range remaining {
			if strings.HasPrefix(filename, "-") {
				continue
			}

			file := linker.MustNewFile(filename)
			ctx.Args.Emulation =
				linker.GetMachineTypeFromContents(file.Contents)
			if ctx.Args.Emulation != linker.MachineTypeNone {
				break
			}
		}
	}

	if ctx.Args.Emulation != linker.MachineTypeRISCV64 {
		utils.Fatal("unknown emulation type")
	}

	// 从命令行中根据 .o 或者 .a 将 obj 文件转化为 ObjectFile 并添加到 Context::Objs 容器中
	// 并且这里所有的符号也创建好了
	// LOCAL 符号对象放在 InputFile::LocalSymbols 中
 	// GLOBAL 符号对象放在 Context::SymbolMap 中
	// 每个 ObjectFile::Symbols 以指针形式指向这些符号对象
	// LOCAL 符号在这里实际上已经 resolve 了，见 ObjectFile::InitializeSymbols
	// GLOBAL 符号此时还没有 resolve
	linker.ReadInputFiles(ctx, remaining)
	
	// 这里调用的是 pkg/linker/passes.go 中的 ResolveSymbols 函数
	// 这一步做完后所有的符号，包括 LOCAL 和 GLOBAL 的符号的符号引用都 resolve 完毕
	// 同时这一步中也完成了 MarkLiveObjects 的操作，即所有需要链接的 obj 文件都被标识出来
	// FIXME：感觉 MarkLiveObjects 可以独立出来作为单独的一步会比较清楚。
	linker.ResolveSymbols(ctx)

	// merge section 相关处理
	// 对每个 obj 文件遍历一遍，将这些 obj 文件中的 mergeable 的 section 中的
	// 重复的 fragment 找出来放到 merged section 中。
	// FIXME：RegisterSectionPieces 中还有一段对符号进行去重的操作，没看懂，TBD
	linker.RegisterSectionPieces(ctx)

	// merge section 相关处理
	// 为 merged section 计算对应的 phdr 的 size 以及 align 信息。
	linker.ComputeMergedSectionSizes(ctx)
	// FIXME: 感觉上面两个 RegisterSectionPieces 和 ComputeMergedSectionSizes
	// 如果都是和 merged section 相关的处理，应该放在一个函数中作为一个大的 step

	// Synthetic：合成的
	// 其实就是对 Context 中的 Chunks 进行初始化，
	// Output 中有四大块：
	// Ehdr/Phdr/Shdr/GotSection
	linker.CreateSyntheticSections(ctx)

	// 对 Context::OutputSections 进行预处理
	linker.BinSections(ctx)

	// 我目前的理解这里是为 output 文件中的 section 部分做准备
	// 收集的 outputsection 来自两部分
	// - Context::OutputSections
	// - Context::MergedSections
	// 搜集好后，还是加入 Context::Chunks，这个动作和上面 CreateSyntheticSections
	// 的动作好像，所以我也认为这些部分应该都属于为 output 做准备
	ctx.Chunks = append(ctx.Chunks, linker.CollectOutputSections(ctx)...)

	// 处理 NeedsGotTp
	linker.ScanRelocations(ctx)

	// 计算 output section 的 shdr 的 size/align 信息
	linker.ComputeSectionSizes(ctx)

	// 对输出的 output section 进行排序
	linker.SortOutputSections(ctx)

	for _, chunk := range ctx.Chunks {
		chunk.UpdateShdr(ctx)
	}

	// 获取最终文件的大小，这样后面 Ctx.Buf 的空间就可以提前申请下来，
	// 为后面的 CopyBuf 做好准备。
	fileSize := linker.SetOutputSectionOffsets(ctx)

	// 为 Context 分配写出 output 的内存
	ctx.Buf = make([]byte, fileSize)

	// 创建 output 文件
	file, err := os.OpenFile(ctx.Args.Output, os.O_RDWR|os.O_CREATE, 0777)
	utils.MustNo(err)

	// 将 chunk 写入 Context 的 buf
	for _, chunk := range ctx.Chunks {
		chunk.CopyBuf(ctx)
	}

	// 最后将 Contxt 的 buf 中的内容写入 output 文件
	_, err = file.Write(ctx.Buf)
	utils.MustNo(err)
}

func parseArgs(ctx *linker.Context) []string {
	args := os.Args[1:]

	dashes := func(name string) []string {
		if len(name) == 1 {
			return []string{"-" + name}
		}
		return []string{"-" + name, "--" + name}
	}

	// readArg 是处理形如 "-o a.out", 即选项后面有参数的形式的
	arg := ""
	readArg := func(name string) bool {
		for _, opt := range dashes(name) {
			if args[0] == opt {
				if len(args) == 1 {
					utils.Fatal(fmt.Sprintf("option -%s: argument missing", name))
				}

				arg = args[1]
				args = args[2:]
				return true
			}

			prefix := opt
			if len(name) > 1 {
				prefix += "="
			}
			if strings.HasPrefix(args[0], prefix) {
				arg = args[0][len(prefix):]
				args = args[1:]
				return true
			}
		}

		return false
	}

	// readFlag 是处理形如 "-v" 只有选项，后面没有参数的
	readFlag := func(name string) bool {
		for _, opt := range dashes(name) {
			if args[0] == opt {
				args = args[1:]
				return true
			}
		}

		return false
	}

	// 可以识别的（包括忽略的）解析后进入 Context::Args，
	// 剩下加入到 remaining 的就是一些形如 "xx.o"（obj 文件） 和 "-lc"（archive 文件）
	remaining := make([]string, 0)
	for len(args) > 0 {
		if readFlag("help") {
			fmt.Printf("usage: %s [options] file...\n", os.Args[0])
			os.Exit(0)
		}

		if readArg("o") || readArg("output") {
			ctx.Args.Output = arg
		} else if readFlag("v") || readFlag("version") {
			fmt.Printf("rvld %s\n", version)
			os.Exit(0)
		} else if readArg("m") {
			if arg == "elf64lriscv" {
				ctx.Args.Emulation = linker.MachineTypeRISCV64
			} else {
				utils.Fatal(fmt.Sprintf("unknown -m argument: %s", arg))
			}
		} else if readArg("L") {
			ctx.Args.LibraryPaths = append(ctx.Args.LibraryPaths, arg)
		} else if readArg("l") {
			remaining = append(remaining, "-l"+arg)
		} else if readArg("sysroot") ||
			readFlag("static") ||
			readArg("plugin") ||
			readArg("plugin-opt") ||
			readFlag("as-needed") ||
			readFlag("start-group") ||
			readFlag("end-group") ||
			readArg("hash-style") ||
			readArg("build-id") ||
			readFlag("s") ||
			readFlag("no-relax") {
			// Ignored
		} else {
			if args[0][0] == '-' {
				utils.Fatal(fmt.Sprintf(
					"unknown command line option: %s", args[0]))
			}
			remaining = append(remaining, args[0])
			args = args[1:]
		}
	}

	for i, path := range ctx.Args.LibraryPaths {
		ctx.Args.LibraryPaths[i] = filepath.Clean(path)
	}

	return remaining
}
