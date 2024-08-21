package linker

import (
	"debug/elf"
	"github.com/ksco/rvld/pkg/utils"
	"sort"
)

/*
 * 见 
 * - https://docs.oracle.com/en/operating-systems/solaris/oracle-solaris/11.4/linkers-libraries/section-merging.html
 * - https://blog.csdn.net/qq_42570601/article/details/124695589 Elf_Shdr::sh_flag 取值为 SHF_XXX
 * 
 */
type MergedSection struct {
	Chunk
	Map map[string]*SectionFragment
}

func NewMergedSection(
	name string, flags uint64, typ uint32) *MergedSection {
	m := &MergedSection{
		Chunk: NewChunk(),
		Map:   make(map[string]*SectionFragment),
	}

	m.Name = name
	m.Shdr.Flags = flags
	m.Shdr.Type = typ
	return m
}

func GetMergedSectionInstance(
	ctx *Context, name string, typ uint32, flags uint64) *MergedSection {
	name = GetOutputName(name, flags)
	
	// FIXME: SHF_MERGE 与 SHF_STRINGS 或许尚可理解
	// 但是为何要扯上 SHF_GROUP 和 SHF_COMPRESSED？
	flags = flags & ^uint64(elf.SHF_GROUP) & ^uint64(elf.SHF_MERGE) &
		^uint64(elf.SHF_STRINGS) & ^uint64(elf.SHF_COMPRESSED)

	// 根据 name、flags 和 type 三个属性寻找相同的 merged section
	find := func() *MergedSection {
		for _, osec := range ctx.MergedSections {
			if name == osec.Name && flags == osec.Shdr.Flags &&
				typ == osec.Shdr.Type {
				return osec
			}
		}

		return nil
	}

	// 如果找到就直接返回这个
	if osec := find(); osec != nil {
		return osec
	}

	// 否则就新建一个 merged section
	osec := NewMergedSection(name, flags, typ)
	ctx.MergedSections = append(ctx.MergedSections, osec)
	return osec
}

func (m *MergedSection) Insert(
	key string, p2align uint32) *SectionFragment {
	frag, ok := m.Map[key]
	if !ok {
		frag = NewSectionFragment(m)
		m.Map[key] = frag
	}

	if frag.P2Align < p2align {
		frag.P2Align = p2align
	}

	return frag
}

func (m *MergedSection) AssignOffsets() {
	var fragments []struct {
		Key string
		Val *SectionFragment
	}

	for key := range m.Map {
		fragments = append(fragments, struct {
			Key string
			Val *SectionFragment
		}{Key: key, Val: m.Map[key]})
	}

	sort.SliceStable(fragments, func(i, j int) bool {
		x := fragments[i]
		y := fragments[j]
		if x.Val.P2Align != y.Val.P2Align {
			return x.Val.P2Align < y.Val.P2Align
		}
		if len(x.Key) != len(y.Key) {
			return len(x.Key) < len(y.Key)
		}

		return x.Key < y.Key
	})

	offset := uint64(0)
	p2align := uint64(0)
	for _, frag := range fragments {
		offset = utils.AlignTo(offset, 1<<frag.Val.P2Align)
		frag.Val.Offset = uint32(offset)
		offset += uint64(len(frag.Key))
		if p2align < uint64(frag.Val.P2Align) {
			p2align = uint64(frag.Val.P2Align)
		}
	}

	m.Shdr.Size = utils.AlignTo(offset, 1<<p2align)
	m.Shdr.AddrAlign = 1 << p2align
}

func (m *MergedSection) CopyBuf(ctx *Context) {
	buf := ctx.Buf[m.Shdr.Offset:]
	for key := range m.Map {
		if frag, ok := m.Map[key]; ok {
			copy(buf[frag.Offset:], key)
		}
	}
}
