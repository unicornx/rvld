package linker

import (
	"github.com/ksco/rvld/pkg/utils"
	"os"
)

// Name: 文件的 name
// Contents：文件的 rawdata
// Parent：当一个 obj 文件归属于一个 archive 文件时，这个 Parent 会指向 archive 文件
//         FIXME：但只看到赋值，没有看到使用的地方。
type File struct {
	Name     string
	Contents []byte
	Parent   *File
}

func MustNewFile(filename string) *File {
	contents, err := os.ReadFile(filename)
	utils.MustNo(err)
	return &File{
		Name:     filename,
		Contents: contents,
	}
}

func OpenLibrary(filepath string) *File {
	contents, err := os.ReadFile(filepath)
	if err != nil {
		return nil
	}

	return &File{
		Name:     filepath,
		Contents: contents,
	}
}

func FindLibrary(ctx *Context, name string) *File {
	for _, dir := range ctx.Args.LibraryPaths {
		stem := dir + "/lib" + name + ".a"
		if f := OpenLibrary(stem); f != nil {
			return f
		}
	}

	utils.Fatal("library not found")
	return nil
}
