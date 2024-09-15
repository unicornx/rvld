package linker

import (
	"debug/elf"
	"github.com/ksco/rvld/pkg/utils"
	"sort"
)

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

// Context::MergedSections 保存了所有处理过后的 merged section
// 在处理 merged section 时我们需要合并，合并的原理就是如果一个同一类的 merged section
// 已经创建过，我们只要做 merge，而不是再建一个。
// 所谓同一类，就是根据 section 的 name/flags/type 都相同

// FIXME 这个函数是不是应该放到 Context 里去呢？
func GetMergedSectionInstance(
	ctx *Context, name string, typ uint32, flags uint64) *MergedSection {
	name = GetOutputName(name, flags)
	
	// 这里是把我们不希望处理的 flags 的 bit 位清除掉
	// 保证我们最终输出的 section 的 flags 里不会含有这些不应该出现的 bit 位。
	flags = flags & ^uint64(elf.SHF_GROUP) & ^uint64(elf.SHF_MERGE) &
		^uint64(elf.SHF_STRINGS) & ^uint64(elf.SHF_COMPRESSED)

	// 一个本地定义的函数：
	// 根据 name、flags 和 type 三个属性去 Context 中寻找相同的 merged section
	// 找不到就返回 nil
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

	// 确保 merge 后的 section，对齐标准要按照最大的那个对齐。
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
