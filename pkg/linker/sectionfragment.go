package linker

import "math"

// 和 Mergable Section 处理有关
// Mergable section 处理过程中会将其 split 成多个 fragment
//
// @OutputSection: 每个 MergedSection 由多个 SectionFragment 组成
//                 通过一个 map 维护在 MergedSection 结构体中
//                 所以每个 SectionFragment 也唯一地属于一个 MergedSection
//                 所以这个 OutputSection 就是该 SectionFragment 所属的
//                 MergedSection。反向指针指向。
// @Offset: 该 fragment 在 section 中的位置
// @IsAlive: 这个是 Fragment 级别的 IsAlive 标记
//           我们现在一共看到有三个级别：
//           InputFile / InputSection / SectionFragment
type SectionFragment struct {
	OutputSection *MergedSection
	Offset        uint32
	P2Align       uint32
	IsAlive       bool
}

// 注意 new 一个 SectionFragment 时，属性基本上都没有赋初值，应该后面要进一步处理
func NewSectionFragment(m *MergedSection) *SectionFragment {
	return &SectionFragment{
		OutputSection: m,
		Offset:        math.MaxUint32, // 初始化为非法值
	}
}

func (s *SectionFragment) GetAddr() uint64 {
	return s.OutputSection.Shdr.Addr + uint64(s.Offset)
}
