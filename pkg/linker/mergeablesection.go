package linker

import "sort"

// MergeableSection 用于存放 splt 处理后的 mergeable 的 InputSection 中的元素
// 所谓 Mergeable Section 即带有 SHF_MERGE 标志的输入的 section
// @Parent:
// @P2Align:
// @Strs: 一个数组，用于存放分割（split）后的元素
//        如果元素的类型是字符串，则这里存放的是分割（split）后的字符串
//        如果元素的类型是固定长度，也存放在这里
// @FragOffsets:rawdata 中每个元素（这里代码中叫 Fragment）的起始偏移量
// @Fragments:
type MergeableSection struct {
	Parent      *MergedSection
	P2Align     uint8
	Strs        []string
	FragOffsets []uint32
	Fragments   []*SectionFragment
}

func (m *MergeableSection) GetFragment(offset uint32) (*SectionFragment, uint32) {
	pos := sort.Search(len(m.FragOffsets), func(i int) bool {
		return offset < m.FragOffsets[i]
	})

	if pos == 0 {
		return nil, 0
	}

	idx := pos - 1
	return m.Fragments[idx], offset - m.FragOffsets[idx]
}
