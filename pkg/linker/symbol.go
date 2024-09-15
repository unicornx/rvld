package linker

import "github.com/ksco/rvld/pkg/utils"

const (
	NeedsGotTp uint32 = 1 << 0
)

/*
 * 用于 linker 内部处理的符号对象，和 ELF 的 Elf_Sym 有一一对应关系，但是 Symbol
 * 对象含有 linker 内部处理需要的上下文信息
 * @File: 标志该 Symbol 属于哪个 ObjectFile，即在哪个文件中定义的
 * @Name：符号的字符串值
 * @Value: Elf_Sym::st_value
 * @SymIdx: 符号在符号表中的index
 *
 * @InputSection: 符号所对应的 InputSection（linker 域）
 * @SectionFragment: 符号也可能属于一个 MergableSection中的 SectionFragment
 *                   具体的例子按照课程视频的说明，假如代码中一个字符串也可能
 *                   会对应到一个符号。FIXME， 具体例子还要看看
 * 上面两个属性同时只有一个为 !nul
 * 这个设计和处理 mergable section 有关
 * 因为根据课程的设计，如果一个 InputSection 是 mergable 的，那么它会被
 * 转化为一个 MergeableSection，而原来的 InputSection 经转化后就无效了
 * FIXME 这个设计其实有点不好理解，其实 MergeableSection 似乎也应该是一个 InputSection
 * 但现在的设计似乎把两者对立起来了。
 */
type Symbol struct {
	File     *ObjectFile
	Name     string
	Value    uint64
	SymIdx   int
	GotTpIdx int32

	InputSection    *InputSection
	SectionFragment *SectionFragment

	Flags uint32
}

func NewSymbol(name string) *Symbol {
	s := &Symbol{
		Name:   name,
		SymIdx: -1,
	}
	return s
}

// 如果一个 Symbol 属于一个 InputSection
func (s *Symbol) SetInputSection(isec *InputSection) {
	s.InputSection = isec
	s.SectionFragment = nil
}

// 如果一个 Symbol 属于一个 SectionFragment
func (s *Symbol) SetSectionFragment(frag *SectionFragment) {
	s.InputSection = nil
	s.SectionFragment = frag
}

func GetSymbolByName(ctx *Context, name string) *Symbol {
	if sym, ok := ctx.SymbolMap[name]; ok {
		return sym
	}
	ctx.SymbolMap[name] = NewSymbol(name)
	return ctx.SymbolMap[name]
}

func (s *Symbol) ElfSym() *Sym {
	utils.Assert(s.SymIdx < len(s.File.ElfSyms))
	return &s.File.ElfSyms[s.SymIdx]
}

func (s *Symbol) Clear() {
	s.File = nil
	s.InputSection = nil
	s.SymIdx = -1
}

func (s *Symbol) GetAddr() uint64 {
	if s.SectionFragment != nil {
		return s.SectionFragment.GetAddr() + s.Value
	}

	if s.InputSection != nil {
		return s.InputSection.GetAddr() + s.Value
	}

	return s.Value
}

func (s *Symbol) GetGotTpAddr(ctx *Context) uint64 {
	return ctx.Got.Shdr.Addr + uint64(s.GotTpIdx)*8
}
