package linker

import (
	"debug/elf"
	"github.com/ksco/rvld/pkg/utils"
	"math"
	"math/bits"
)

/*
 * 和 ELF 文件中的 Elf Section 一一对应的 InputSection 对象，用于 linker
 * 内部的处理
 * @File: 含有该 section 的 ObjectFile 对象
 * @Contents: 该 section 的 rawdata
 * @Shndx: 该 section 在 section header table 中的 index
 * @ShSize: 该 section rawdata 的 size，虽然一个 inputsection 对应一个 elf section
 *          但是最终的 InputSection 的 size 会被改动，所以另外定一个 Inputsection 自己的 size 成员。
 * @IsAlive: Section 级别的 isAlive 标记，而 InputFile::IsAlive 是文件级别的 isAlive 标记
 *           需要找个属性的原因是，即使一个文件是 isAlive 的（需要输出到 output 文件中），
 *           但并不意味着找个文件中所有的 section 都需要输出到 output 文件中。
 *           FIXME：
 *           标识这个 mergebale 的 InputSection 是否已经被 split 处理过？
 *           参考 InitializeMergeableSections()
 *           标识这个 input section 是 ".eh_frame" 的
 * @P2Align: Elf_hdr::sh_addralign 中存放的表示地址对齐的 2 的指数值。
 *           P2Align 则将其转化为指数值，譬如 sh_addralign = 4 时对应的 P2Align 为 2
 *           sh_addralign = 8 时对应的 P2Align 为 3
 * @Offset:
 * @OutputSection: 该 input section 对应的 output section
 * @RelsecIdx: 对于类型为 SHT_RELA 的 section（重定位表），这个属性存放了该重定位
 *             表的 section 在 section table 中的 index
 * @Rels: 
 */
type InputSection struct {
	File     *ObjectFile
	Contents []byte
	Shndx    uint32
	ShSize   uint32
	IsAlive  bool
	P2Align  uint8

	Offset        uint32
	OutputSection *OutputSection

	RelsecIdx uint32
	Rels      []Rela
}

// 根据一个 ELF section 创建一个 InputSection
func NewInputSection(ctx *Context, name string, file *ObjectFile, shndx uint32) *InputSection {
	s := &InputSection{
		File:      file, // InputSection::File
		Shndx:     shndx,// InputSection::Shndx
		IsAlive:   true, // 注意这里 InputSection::IsAlive 默认为 true，后面判断不需要输出时再标记为 false
		Offset:    math.MaxUint32,
		RelsecIdx: math.MaxUint32,
		ShSize:    math.MaxUint32,
	}

	// 填写 InputSection::Contexts 的内容，以备后面使用
	// 找到这个 InputSection 对应的 ELf section header
	shdr := s.Shdr()
	s.Contents = file.File.Contents[shdr.Offset : shdr.Offset+shdr.Size]

	// InputSection::ShSize， 这个和 InputSection::Contexts 是配套使用的
	// 课程中暂不支持压缩的方式，所以这里只是 assert 一下。
	utils.Assert(shdr.Flags&uint64(elf.SHF_COMPRESSED) == 0)
	s.ShSize = uint32(shdr.Size)

	// InputSection::P2Align
	toP2Align := func(align uint64) uint8 {
		if align == 0 {
			return 0
		}
		return uint8(bits.TrailingZeros64(align))
	}
	s.P2Align = toP2Align(shdr.AddrAlign)

	// InputSection::OutputSection
	// InputSection 和 OutputSection 之间是多对一的关系
	// 这里我们通过 GetOutputSection 找到当前这个 InputSection
	// 对应的 OutputSection。
	// GetOutputSection 中隐含了向 Context 注册新的 OutputSection
	// 的动作，所以我们对所有的 InputSection 都执行完 NewInputSection
	// 后 Context::OutputSections 就包含了所有我们需要输出的 section 项目。
	s.OutputSection = GetOutputSection(
		ctx, name, uint64(shdr.Type), shdr.Flags)

	return s
}

func (i *InputSection) Shdr() *Shdr {
	utils.Assert(i.Shndx < uint32(len(i.File.ElfSections)))
	return &i.File.ElfSections[i.Shndx]
}

func (i *InputSection) Name() string {
	return ElfGetName(i.File.ShStrtab, i.Shdr().Name)
}

func (i *InputSection) WriteTo(ctx *Context, buf []byte) {
	// .bss 就是 SHT_NOBITS
	if i.Shdr().Type == uint32(elf.SHT_NOBITS) || i.ShSize == 0 {
		return
	}

	i.CopyContents(buf)

	if i.Shdr().Flags&uint64(elf.SHF_ALLOC) != 0 {
		i.ApplyRelocAlloc(ctx, buf)
	}
}

func (i *InputSection) CopyContents(buf []byte) {
	copy(buf, i.Contents)
}

func (i *InputSection) GetRels() []Rela {
	// FIXME: 对 i.Rels 的判断有何作用？什么情况下会走这个分支？
	// 估计是 GetRels 这个函数会被两处调用：
	// 一处是 ScanRelocations，另一处是 ApplyRelocAlloc
	if i.RelsecIdx == math.MaxUint32 || i.Rels != nil {
		return i.Rels
	}

	// 将重定位表数据从其所在 section 中读取出来然后分片
	bs := i.File.GetBytesFromShdr(
		&i.File.InputFile.ElfSections[i.RelsecIdx])
	i.Rels = utils.ReadSlice[Rela](bs, RelaSize)
	// 最后形成 Rels 的数组后返回。
	return i.Rels
}

func (i *InputSection) GetAddr() uint64 {
	return i.OutputSection.Shdr.Addr + uint64(i.Offset)
}

func (i *InputSection) ScanRelocations() {
	for _, rel := range i.GetRels() {
		sym := i.File.Symbols[rel.Sym]
		if sym.File == nil {
			continue
		}

		if rel.Type == uint32(elf.R_RISCV_TLS_GOT_HI20) {
			sym.Flags |= NeedsGotTp
		}
	}
}

func (i *InputSection) ApplyRelocAlloc(ctx *Context, base []byte) {
	rels := i.GetRels()

	for a := 0; a < len(rels); a++ {
		rel := rels[a]
		if rel.Type == uint32(elf.R_RISCV_NONE) ||
			rel.Type == uint32(elf.R_RISCV_RELAX) {
			continue
		}

		sym := i.File.Symbols[rel.Sym]
		loc := base[rel.Offset:]

		if sym.File == nil {
			continue
		}

		S := sym.GetAddr()
		A := uint64(rel.Addend)
		P := i.GetAddr() + rel.Offset

		switch elf.R_RISCV(rel.Type) {
		case elf.R_RISCV_32:
			utils.Write[uint32](loc, uint32(S+A))
		case elf.R_RISCV_64:
			utils.Write[uint64](loc, S+A)
		case elf.R_RISCV_BRANCH:
			writeBtype(loc, uint32(S+A-P))
		case elf.R_RISCV_JAL:
			writeJtype(loc, uint32(S+A-P))
		case elf.R_RISCV_CALL, elf.R_RISCV_CALL_PLT:
			val := uint32(S + A - P)
			writeUtype(loc, val)
			writeItype(loc[4:], val)
		case elf.R_RISCV_TLS_GOT_HI20:
			utils.Write[uint32](loc, uint32(sym.GetGotTpAddr(ctx)+A-P))
		case elf.R_RISCV_PCREL_HI20:
			utils.Write[uint32](loc, uint32(S+A-P))
		case elf.R_RISCV_HI20:
			writeUtype(loc, uint32(S+A))
		case elf.R_RISCV_LO12_I, elf.R_RISCV_LO12_S:
			val := S + A
			if rel.Type == uint32(elf.R_RISCV_LO12_I) {
				writeItype(loc, uint32(val))
			} else {
				writeStype(loc, uint32(val))
			}

			if utils.SignExtend(val, 11) == val {
				setRs1(loc, 0)
			}
		case elf.R_RISCV_TPREL_LO12_I, elf.R_RISCV_TPREL_LO12_S:
			val := S + A - ctx.TpAddr
			if rel.Type == uint32(elf.R_RISCV_TPREL_LO12_I) {
				writeItype(loc, uint32(val))
			} else {
				writeStype(loc, uint32(val))
			}

			if utils.SignExtend(val, 11) == val {
				setRs1(loc, 4)
			}
		}
	}

	for a := 0; a < len(rels); a++ {
		switch elf.R_RISCV(rels[a].Type) {
		case elf.R_RISCV_PCREL_LO12_I, elf.R_RISCV_PCREL_LO12_S:
			sym := i.File.Symbols[rels[a].Sym]
			utils.Assert(sym.InputSection == i)
			loc := base[rels[a].Offset:]
			val := utils.Read[uint32](base[sym.Value:])

			if rels[a].Type == uint32(elf.R_RISCV_PCREL_LO12_I) {
				writeItype(loc, val)
			} else {
				writeStype(loc, val)
			}
		}
	}

	for a := 0; a < len(rels); a++ {
		switch elf.R_RISCV(rels[a].Type) {
		case elf.R_RISCV_PCREL_HI20, elf.R_RISCV_TLS_GOT_HI20:
			loc := base[rels[a].Offset:]
			val := utils.Read[uint32](loc)
			utils.Write[uint32](loc, utils.Read[uint32](i.Contents[rels[a].Offset:]))
			writeUtype(loc, val)
		}
	}
}

func itype(val uint32) uint32 {
	return val << 20
}

func stype(val uint32) uint32 {
	return utils.Bits(val, 11, 5)<<25 | utils.Bits(val, 4, 0)<<7
}

func btype(val uint32) uint32 {
	return utils.Bit(val, 12)<<31 | utils.Bits(val, 10, 5)<<25 |
		utils.Bits(val, 4, 1)<<8 | utils.Bit(val, 11)<<7
}

func utype(val uint32) uint32 {
	return (val + 0x800) & 0xffff_f000
}

func jtype(val uint32) uint32 {
	return utils.Bit(val, 20)<<31 | utils.Bits(val, 10, 1)<<21 |
		utils.Bit(val, 11)<<20 | utils.Bits(val, 19, 12)<<12
}

func cbtype(val uint16) uint16 {
	return utils.Bit(val, 8)<<12 | utils.Bit(val, 4)<<11 | utils.Bit(val, 3)<<10 |
		utils.Bit(val, 7)<<6 | utils.Bit(val, 6)<<5 | utils.Bit(val, 2)<<4 |
		utils.Bit(val, 1)<<3 | utils.Bit(val, 5)<<2
}

func cjtype(val uint16) uint16 {
	return utils.Bit(val, 11)<<12 | utils.Bit(val, 4)<<11 | utils.Bit(val, 9)<<10 |
		utils.Bit(val, 8)<<9 | utils.Bit(val, 10)<<8 | utils.Bit(val, 6)<<7 |
		utils.Bit(val, 7)<<6 | utils.Bit(val, 3)<<5 | utils.Bit(val, 2)<<4 |
		utils.Bit(val, 1)<<3 | utils.Bit(val, 5)<<2
}

func writeItype(loc []byte, val uint32) {
	mask := uint32(0b000000_00000_11111_111_11111_1111111)
	utils.Write[uint32](loc, (utils.Read[uint32](loc)&mask)|itype(val))
}

func writeStype(loc []byte, val uint32) {
	mask := uint32(0b000000_11111_11111_111_00000_1111111)
	utils.Write[uint32](loc, (utils.Read[uint32](loc)&mask)|stype(val))
}

func writeBtype(loc []byte, val uint32) {
	mask := uint32(0b000000_11111_11111_111_00000_1111111)
	utils.Write[uint32](loc, (utils.Read[uint32](loc)&mask)|btype(val))
}

func writeUtype(loc []byte, val uint32) {
	mask := uint32(0b000000_00000_00000_000_11111_1111111)
	utils.Write[uint32](loc, (utils.Read[uint32](loc)&mask)|utype(val))
}

func writeJtype(loc []byte, val uint32) {
	mask := uint32(0b000000_00000_00000_000_11111_1111111)
	utils.Write[uint32](loc, (utils.Read[uint32](loc)&mask)|jtype(val))
}

func setRs1(loc []byte, rs1 uint32) {
	utils.Write[uint32](loc, utils.Read[uint32](loc)&0b111111_11111_00000_111_11111_1111111)
	utils.Write[uint32](loc, utils.Read[uint32](loc)|(rs1<<15))
}
