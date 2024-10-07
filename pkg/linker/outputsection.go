package linker

import "debug/elf"

// @Members：一个 OutputSection 对应多个“同名”的 InputSection
//           FIXME：没有看懂的是，代码中如何保证 OutputSection 对应的 InputSection 是同一个类型的？
// @Idx: 本 outputsection 在 ctx.OutputSections 数组中的下标
//       这个值的确定在 GetOutputSection() 中最后 NewOutputSection() 时传入的第三个参数
//       uint32(len(ctx.OutputSections))
type OutputSection struct {
	Chunk
	Members []*InputSection
	Idx     uint32
}

func NewOutputSection(
	name string, typ uint32, flags uint64, idx uint32) *OutputSection {
	o := &OutputSection{Chunk: NewChunk()}
	o.Name = name
	o.Shdr.Type = typ
	o.Shdr.Flags = flags
	o.Idx = idx
	return o
}

func (o *OutputSection) CopyBuf(ctx *Context) {
	if o.Shdr.Type == uint32(elf.SHT_NOBITS) {
		return
	}

	base := ctx.Buf[o.Shdr.Offset:]
	for _, isec := range o.Members {
		isec.WriteTo(ctx, base[isec.Offset:])
	}
}

// 返回值是一个 OutputSection 的指针
func GetOutputSection(
	ctx *Context, name string, typ, flags uint64) *OutputSection {
	// 根据 InputSection 的 name 和 flags，映射获得对应的 OutputSection 的名字
	name = GetOutputName(name, flags)
	// FIXME
	flags = flags &^ uint64(elf.SHF_GROUP) &^
		uint64(elf.SHF_COMPRESSED) &^ uint64(elf.SHF_LINK_ORDER)

	// 内嵌定义个 find 函数
	find := func() *OutputSection {
		for _, osec := range ctx.OutputSections {
			if name == osec.Name && typ == uint64(osec.Shdr.Type) &&
				flags == osec.Shdr.Flags {
				return osec
			}
		}
		return nil
	}

	// 调用内嵌定义的 find 函数，搜索一下 Context::OutputSections 中是否
	// 已经注册了这个名字的 OutputSection
	// 如果找到就返回
	if osec := find(); osec != nil {
		return osec
	}

	// 如果没有找到，就用这个名字新注册一个到 Context::OutputSections 中再
	// 返回
	osec := NewOutputSection(name, uint32(typ), flags,
		uint32(len(ctx.OutputSections)))
	ctx.OutputSections = append(ctx.OutputSections, osec)
	return osec
}
