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
 * @OutputSections
 * @Chunks
 * @Objs
 * @SymbolMap
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
