package linker

import (
	"debug/elf"
	"strings"
)

// 链接过程中，linker 的输入是 N 个 ObjectFile，而输出只有一个文件
// 所以我们需要将多个输入的 ObjectFile 中 InputSection 进行合并。
// 譬如 .text 合并成一个 .text
// 这都需要通过 section 的名字来进行判断和处理


var prefixes = []string{
	".text.", ".data.rel.ro.", ".data.", ".rodata.", ".bss.rel.ro.", ".bss.",
	".init_array.", ".fini_array.", ".tbss.", ".tdata.", ".gcc_except_table.",
	".ctors.", ".dtors.",
}

// 输入的是 inputsection 的 name ，输出的是 outputsection 的名字
// 获取允我们支持的在 output 文件中允许出现的 section 的名字
func GetOutputName(name string, flags uint64) string {
	// 针对 Mergable section 的特殊处理
	// Mergeable section 一定是 readonly 的
	if (name == ".rodata" || strings.HasPrefix(name, ".rodata.")) &&
		flags&uint64(elf.SHF_MERGE) != 0 {
		// mergable section 中的内容有两种
		// 一种是含有字符串
		// 一种是常量
		if flags&uint64(elf.SHF_STRINGS) != 0 {
			return ".rodata.str"
		} else {
			return ".rodata.cst"
		}
	}

	for _, prefix := range prefixes {
		// 把 prefixes 表中字符串的最后一个 "." 去掉
		// FIXME：有必要吗，原表中直接先处理去掉不好吗？
		// A: 不好，因为我们也会用到 prefix
		stem := prefix[:len(prefix)-1]
		if name == stem || strings.HasPrefix(name, prefix) {
			return stem
		}
	}

	return name
}
