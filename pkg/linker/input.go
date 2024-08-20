package linker

import "github.com/ksco/rvld/pkg/utils"

// 遍历并处理命令行中 remaining 的部分，也就是除去 option 选项后剩下的部分，主要是
// .o 文件或者 -lxx（archive 文件）
// 对于 .o 文件，直接处理后转化为 ObjectFile 类型并加入 Context::Objs 中
// 对于 archive 文件，提取出其中的 .o 文件后同样转化为 ObjectFile 类型并加入 Context::Objs 中
func ReadInputFiles(ctx *Context, remaining []string) {
	for _, arg := range remaining {
		var ok bool
		if arg, ok = utils.RemovePrefix(arg, "-l"); ok {
			// FindLibrary 会确保根据 “-L” 选项指定的路径下去搜索 .a 文件
			ReadFile(ctx, FindLibrary(ctx, arg))
		} else {
			ReadFile(ctx, MustNewFile(arg))
		}
	}
}

func ReadFile(ctx *Context, file *File) {
	ft := GetFileType(file.Contents)
	switch ft {
	case FileTypeObject:
		ctx.Objs = append(ctx.Objs, CreateObjectFile(ctx, file, false))
	case FileTypeArchive:
		for _, child := range ReadArchiveMembers(file) {
			utils.Assert(GetFileType(child.Contents) == FileTypeObject)
			ctx.Objs = append(ctx.Objs, CreateObjectFile(ctx, child, true))
		}
	default:
		utils.Fatal("unknown file type")
	}
}

func CreateObjectFile(ctx *Context, file *File, inLib bool) *ObjectFile {
	// 确保打开的是 RISCV 文件
	CheckFileCompatibility(ctx, file)

	// 创建 ObjectFile 对象时，
	// 对 FileTypeObject 的 .o 默认 alive 都设置为 true
	// 对 FileTypeArchive 中的 .o 默认 alive 为 false
	// alive 说明需要加入最终的 output
	obj := NewObjectFile(file, !inLib)
	// 
	obj.Parse(ctx)
	return obj
}
