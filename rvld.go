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

	
	linker.RegisterSectionPieces(ctx)

	linker.ComputeMergedSectionSizes(ctx)

	linker.CreateSyntheticSections(ctx)

	linker.BinSections(ctx)

	ctx.Chunks = append(ctx.Chunks, linker.CollectOutputSections(ctx)...)

	linker.ScanRelocations(ctx)

	linker.ComputeSectionSizes(ctx)

	linker.SortOutputSections(ctx)

	for _, chunk := range ctx.Chunks {
		chunk.UpdateShdr(ctx)
	}

	fileSize := linker.SetOutputSectionOffsets(ctx)

	ctx.Buf = make([]byte, fileSize)

	file, err := os.OpenFile(ctx.Args.Output, os.O_RDWR|os.O_CREATE, 0777)
	utils.MustNo(err)

	for _, chunk := range ctx.Chunks {
		chunk.CopyBuf(ctx)
	}

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
