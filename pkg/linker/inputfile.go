package linker

import (
	"debug/elf"
	"fmt"
	"github.com/ksco/rvld/pkg/utils"
)

/*
 * @File
 * @ElfSections: Shdr（ELF Section header）的数组
 * @ElfSyms: Sym（ELF Symbol） 的数组
 * @FirstGlobal
 * @ShStrtab: Section Header 字符串的 rawdata，本质上是一个 byte 数组，内部字符串以 '\0' 结尾
 * @SymbolStrtab: 符号表字符串的 rawdata，其实就是对应的 .strtab section
 * @IsAlive: 标记该文件是否最终要输出到 Output
 * @Symbols: 该文件中所有的符号，包括 LOCAL 和 GLOBAL，注意这里存放的是 Symbol* ，
 *           实际的 LOCAL 符号对象放在下面的 LocalSymbols 中
 *           实际的 GLOBAL 符号对象放在 Context::SymbolMap 中
 *           可以参考 ObjectFile::InitializeSymbols
 *           注意 Symbol 和 Sym 不同，Sym 是 ELF 的概念， Symbol 是 Linker 的概念
 * @LocalSymbols: 该文件的 LOCAL 符号，存放形式是一个 Symbol 的数组
 */
type InputFile struct {
	File         *File  // 由 NewInputFile 解析获取
	ElfSections  []Shdr // 由 NewInputFile 解析获取
	ElfSyms      []Sym  // 由 ObjectFile::Parse 解析获取
	FirstGlobal  int    // 由 ObjectFile::Parse 解析获取
	ShStrtab     []byte // 由 NewInputFile 解析获取
	SymbolStrtab []byte // 由 ObjectFile::Parse 解析获取
	IsAlive      bool   // 由 NewObjectFile 解析获取
	Symbols      []*Symbol
	LocalSymbols []Symbol
}

// 创建 InputFile 对象和初始化
// 包括：
// ElfSections
// ShStrtab
func NewInputFile(file *File) InputFile {
	f := InputFile{File: file}
	// 文件长度小于 Ehdr 的长度是不合理的，至少有一个 Ehdr
	if len(file.Contents) < EhdrSize {
		utils.Fatal("file too small")
	}

	// 检查 Ehdr 的 magic 数
	if !CheckMagic(file.Contents) {
		utils.Fatal("not an ELF file")
	}

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

	shstrndx := int64(ehdr.ShStrndx)
	if ehdr.ShStrndx == uint16(elf.SHN_XINDEX) {
		shstrndx = int64(shdr.Link)
	}
	// 获取 section head 的字符串数组
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
