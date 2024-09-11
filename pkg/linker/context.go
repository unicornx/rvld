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
 *  此外，还值得注意的是：
 *  整个链接过程中涉及的 GLOBAL 符号的对象实体实际上都存放在 Context::SymbolMap 中
 *  每个 ObjectFile::Symbols 成员是 Symbl 指针类型，都是指向整个 map。
 * @MergedSections: 用于保存 Merged 的 Sections
 *                  有关 Merged Section 的概念和 section header entry 中的 SHF_MERGE 的有关，
 *                  参考 https://refspecs.linuxbase.org/elf/gabi4+/ch4.sheader.html
 *                  大致的意思是说，存在以下两种情况：
 *                  情况一，Elf_Shdr::sh_flag 中 SHF_MERGE 比特位被置上，
*                           但是 SHF_STRINGS 没有被设置，此时
 *                          该 section 的内容由多个固定长度的元素构成，
 *                          元素的大小参考 Elf_Shdr::sh_entsize 
 *                  情况二，Elf_Shdr::sh_flag 中 SHF_MERGE 和 SHF_STRINGS 比特位
 *                          都被置上，则该 section 的内容
 *                          由多个以零结尾的字符串构成，注意字符串的每个字符的宽
 *                          度不固定，具体大小由 Elf_Shdr::sh_entsize 的值确定。
 *                  对于以上两种情况的多个输入文件中的 section 的元素内容，如果出现重复的情况可以被合并（merge）
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
