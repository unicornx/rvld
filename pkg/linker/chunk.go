package linker

// Go 语言限制，不支持基类指针，所以用 interface 方式实现
// 所有以 Chunk 为基类的类都需要实现以下的虚函数
type Chunker interface {
	GetName() string
	GetShdr() *Shdr
	UpdateShdr(ctx *Context)
	CopyBuf(ctx *Context)
}

// Chunk 本身作为一个基类
// @Shndx: FIXME，这个成员似乎没有用到
type Chunk struct {
	Name  string
	Shdr  Shdr
	Shndx int64
}

func NewChunk() Chunk {
	// 默认 AddrAilign 为 1，即 1 字节对齐
	return Chunk{Shdr: Shdr{AddrAlign: 1}}
}

func (c *Chunk) GetName() string {
	return c.Name
}

func (c *Chunk) GetShdr() *Shdr {
	return &c.Shdr
}

func (c *Chunk) UpdateShdr(ctx *Context) {}

func (c *Chunk) CopyBuf(ctx *Context) {}
