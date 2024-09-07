package linker

import (
	"debug/elf"
	"fmt"
	"github.com/ksco/rvld/pkg/utils"
)

/*
 * @File
 * @ElfSections: Shdr（ELF Section header）的数组
 * @ShStrtab: Section Header 字符串的 rawdata，本质上是一个 byte 数组，内部字符串以 '\0' 结尾
 * @ElfSyms: ELF 符号表。是 Sym（ELF Symbol）类型的数组
 * @FirstGlobal: ELF 符号表中第一个 Global 符号的位置
 * @SymbolStrtab: ELF 符号表字符串的 rawdata，其实就是对应的 .strtab section
 * @IsAlive: 标记该文件是否最终要输出到 Output
 * @Symbols: 一个 InputFile（这里主要是指 ObjectFile）文件中所有的符号，
 *           包括 LOCAL 和 GLOBAL，经过解析处理后，具体参考 ObjectFile::InitializeSymbols
 *           实际的 LOCAL 符号对象放在 InputFile::LocalSymbols 中
 *           实际的 GLOBAL 符号对象放在 Context::SymbolMap 中
 *           所以这里 Symbols 数组成员类型是 Symbol*，指针指向实际的 Symbol 对象
 *           注意 Symbol 和 Sym 不同，Sym 是 ELF 的概念， Symbol 是 Linker 的概念
 * @LocalSymbols: 该文件的 LOCAL 符号，存放形式是一个 Symbol 的数组
 */
type InputFile struct {
	File         *File  // 由 NewInputFile 解析获取
	ElfSections  []Shdr // 由 NewInputFile 解析获取
	ShStrtab     []byte // 由 NewInputFile 解析获取
	ElfSyms      []Sym  // 由 ObjectFile::Parse 解析获取
	FirstGlobal  int    // 由 ObjectFile::Parse 解析获取
	SymbolStrtab []byte // 由 ObjectFile::Parse 解析获取
	IsAlive      bool   // 由 NewObjectFile 解析获取
	Symbols      []*Symbol
	LocalSymbols []Symbol
}

// 创建 InputFile 对象,初始化后返回这个对象的引用（注意不是指针）
// 包括：
// - File
// - ElfSections
// - ShStrtab
// 这个函数会在创建 ObjectFile 对象的时候被调用，用于初始化 ObjectFile
// 的基类
func NewInputFile(file *File) InputFile {
	///////////////////////////////////////////////
	// 初始化 InputFile::File
	// 这里应该是 go 的语法，InputFile 的成员 File 被赋值
	f := InputFile{File: file}
	
	// 文件长度小于 Ehdr 的长度是不合理的，至少有一个 Ehdr
	if len(file.Contents) < EhdrSize {
		utils.Fatal("file too small")
	}

	// 检查 Ehdr 的 magic 数
	if !CheckMagic(file.Contents) {
		utils.Fatal("not an ELF file")
	}

	///////////////////////////////////////////////
	// 初始化 InputFile::ElfSections
	// 读入 ELF 的 header
	ehdr := utils.Read[Ehdr](file.Contents)
	contents := file.Contents[ehdr.ShOff:]
	// 读入 ELF 的 section header
	shdr := utils.Read[Shdr](contents)

	// 获取 section 的个数
	numSections := int64(ehdr.ShNum)
	if numSections == 0 {
		numSections = int64(shdr.Size)
	}

	// 将 ELF 的 section header 的内容读入，本质上是一个数组，数组的每个成员是 Shdr
	f.ElfSections = []Shdr{shdr}
	for numSections > 1 {
		contents = contents[ShdrSize:]
		f.ElfSections = append(f.ElfSections, utils.Read[Shdr](contents))
		numSections--
	}

	///////////////////////////////////////////////
	// 初始化 InputFile::ShStrtab
	// 获取 section header string section 的 index
	shstrndx := int64(ehdr.ShStrndx)
	// 特殊情况处理，对于 section 个数大于 65535 的情况
	if ehdr.ShStrndx == uint16(elf.SHN_XINDEX) {
		shstrndx = int64(shdr.Link)
	}
	// 获取 section header string section 的 rawdata
	f.ShStrtab = f.GetBytesFromIdx(shstrndx)
	return f
}

func (f *InputFile) GetBytesFromShdr(s *Shdr) []byte {
	end := s.Offset + s.Size
	if uint64(len(f.File.Contents)) < end {
		utils.Fatal(
			fmt.Sprintf("section header is out of range: %d", s.Offset))
	}
	return f.File.Contents[s.Offset:end]
}

func (f *InputFile) GetBytesFromIdx(idx int64) []byte {
	return f.GetBytesFromShdr(&f.ElfSections[idx])
}

func (f *InputFile) FillUpElfSyms(s *Shdr) {
	bs := f.GetBytesFromShdr(s)
	f.ElfSyms = utils.ReadSlice[Sym](bs, SymSize)
}

// 根据 section 的 type 寻找第一个匹配的 section header 
func (f *InputFile) FindSection(ty uint32) *Shdr {
	for i := 0; i < len(f.ElfSections); i++ {
		shdr := &f.ElfSections[i]
		if shdr.Type == ty {
			return shdr
		}
	}

	return nil
}

func (f *InputFile) GetEhdr() Ehdr {
	return utils.Read[Ehdr](f.File.Contents)
}
