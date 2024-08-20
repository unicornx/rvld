package linker

type ContextArgs struct {
	Output       string
	Emulation    MachineType
	LibraryPaths []string
}

/*
 * @Args: 我们感兴趣的一些需要记下来的命令行选项参数值
 * @Buf
 * @Ehdr
 * @Shdr
 * @Phdr
 * @Got
 * @TpAddr
 * @OutputSections: 输出文件中需要产生的 sections
 *                  这些 sections 的创建参考 GetOutputSection()
 *                  在遍历所有输入的 obj 文件的过程中，会触发该函数
 *                  main
 *                  -> ReadInputFiles
 *                     -> ReadFile
 *                        -> CreateObjectFile
 *                           -> Parse
 *                              -> InitializeSections
 *                                 -> NewInputSection
 *                                    -> GetOutputSection
 * @Chunks
 * @Objs: 所有输入文件中的 obj 文件，包括 .o 文件以及 .a 文件中 extracted 的 .o 文件
 * @SymbolMap: 所有输入文件的 GLOBAL 符号。
 *             这些符号的添加动作参考 GetSymbolByName() 函数
 *             在遍历所有输入的 obj 文件的过程中，会触发该函数
  *                  main
 *                  -> ReadInputFiles
 *                     -> ReadFile
 *                        -> CreateObjectFile
 *                           -> Parse
 *                              -> InitializeSymbols
 *                                 -> GetSymbolByName
 * @MergedSections: 用于保存 Merged 的 Sections
 */
type Context struct {
	Args ContextArgs
	Buf  []byte

	Ehdr *OutputEhdr
	Shdr *OutputShdr
	Phdr *OutputPhdr
	Got  *GotSection

	TpAddr uint64

	OutputSections []*OutputSection

	Chunks []Chunker

	Objs           []*ObjectFile
	SymbolMap      map[string]*Symbol
	MergedSections []*MergedSection
}

func NewContext() *Context {
	return &Context{
		Args: ContextArgs{
			Output:    "a.out",
			Emulation: MachineTypeNone,
		},
		SymbolMap: make(map[string]*Symbol),
	}
}
