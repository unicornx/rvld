package linker

import "sort"

// SHF_MERGE
// Identifies a section containing data that may be merged to eliminate duplication.
// Unless the SHF_STRINGS flag is also set, the data elements in the section are
// of a uniform size. The size of each element is specified in the section header's
// sh_entsize field. If the SHF_STRINGS flag is also set, the data elements
// consist of null-terminated character strings. The size of each character is
// specified in the section header's sh_entsize field.
//
// 参考：
// [1]:https://docs.oracle.com/en/operating-systems/solaris/oracle-solaris/11.4/linkers-libraries/section-merging.html
// [2]:https://blog.csdn.net/qq_42570601/article/details/124695589 Elf_Shdr::sh_flag 取值为 SHF_XXX
//
// 摘录自 [1]: 
// The SHF_MERGE section flag can be used to mark SHT_PROGBITS sections within relocatable objects.
// 注：SHT_PROGBITS 标识这个 section 包含了程序需要的数据，格式和含义由程序解释。代码节、数据节都是这种类型。
// This flag indicates that the section can be merged with compatible sections from other objects. 
// A SHF_MERGE flagged section indicates that the section adheres to the following characteristics.
// - The section is read-only. 
// - Every item in the section is accessed from an individual relocation record.
// - If the section also has the SHF_STRINGS flag set, then the section can only contain null terminated strings. 
// SHF_MERGE is an optional flag indicating a possible optimization. The link-editor is allowed to perform the optimization, or to ignore the optimization.

// 参考 https://refspecs.linuxbase.org/elf/gabi4+/ch4.sheader.html
// 摘录：
// 大致的意思是说，存在以下两种情况：
// 情况一，Elf_Shdr::sh_flag 中 SHF_MERGE 比特位被置上，
//         但是 SHF_STRINGS 没有被设置，此时
//         该 section 的内容由多个固定长度的元素构成，
//         元素的大小参考 Elf_Shdr::sh_entsize 
// 情况二，Elf_Shdr::sh_flag 中 SHF_MERGE 和 SHF_STRINGS 比特位
//         都被置上，则该 section 的内容
//         由多个以零结尾的字符串构成，注意字符串的每个字符的宽
//         度不固定，具体大小由 Elf_Shdr::sh_entsize 的值确定。
// 对于以上两种情况的多个输入文件中的 section 的元素内容，如果出现重复的情况可以被合并（merge）

// 课程上（第六课 25:00 左右）说到，对 Mergable section 的处理是要将其拆分，然后再合并，这个概念或许和 split 以及 fragment 的概念有关

// MergeableSection 用于存放 splt 处理后的 mergeable 的 InputSection 中的元素
// 所谓 Mergeable Section 即带有 SHF_MERGE 标志的输入的 section
// 编译器在生成某些内容时，主要是对于一些字符串常量，那么为了节省
// 内存，引用字符串时可以只引用一份，对于这种情况，编译器可能会在不同 obj 种把这个相同的字符串
// 放在一个特殊的 section 种，并将该 section标记为 mergeable。
// 然后交给链接器，链接器在处理的时候对于这些 mergable 的 section只保留一份copy

// @Parent: 每个 MergeableSection 都对应一个 MergedSection
//          MergedSection vs MergableSection 是 1:N 的关系
//          在设计上，存在如下关系：
//          Context 中维护所有 MergedSection，存放在一个数组中，即 Context::MergedSections
//          每个 MergedSection 对应多个 objfile 中的 MergableSection，至于哪些 MergableSection 会
//          被 merge 为一个 MergedSection，这个由 name/flags/types 三元组决定，MergedSection 
//          由函数 splitSection() 中调用 GetMergedSectionInstance() 生成。
//          假设以字符串section为例，所有的 object 文件的 .strtab section
//          在最后输出的可执行文件中就会被 merge 成一个。
// @P2Align:
// @Strs: 一个数组，用于存放分割（split）后的元素
//        如果元素的类型是字符串，则这里存放的是分割（split）后的字符串
//        如果元素的类型是固定长度，也以 string 形式存放在这里
// @FragOffsets: rawdata 中每个元素（这里代码中叫 Fragment）的起始偏移量
// @Fragments: 按照课程的说法，上面的 Strs 和 FragOffsets 是 Fragment 的原始数据，而这里的 Fragments
//             是在原始数据的基础上经过处理后的，处理的方法见 RegisterSectionPieces 函数
type MergeableSection struct {
	Parent      *MergedSection
	P2Align     uint8
	Strs        []string
	FragOffsets []uint32
	Fragments   []*SectionFragment
}

// FIXME, 这个函数的具体含义等看 RegisterSectionPieces 时再细看
func (m *MergeableSection) GetFragment(offset uint32) (*SectionFragment, uint32) {
	// FragOffsets 必定是排好序的，从小到大
	pos := sort.Search(len(m.FragOffsets), func(i int) bool {
		return offset < m.FragOffsets[i]
	})

	if pos == 0 {
		return nil, 0
	}

	idx := pos - 1
	return m.Fragments[idx], offset - m.FragOffsets[idx]
}
