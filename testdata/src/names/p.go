package names

type Named struct { // want `struct has size 24`
	a uint32
	b uint64
	c uint32
}

type Outer struct {
	inner struct { // want `struct has size 24`
		a uint32
		b uint64
		c uint32
	}
	ptrs []*struct { // want `struct has size 24`
		a uint32
		b uint64
		c uint32
	}
}

var global struct { // want `struct has size 24`
	a uint32
	b uint64
	c uint32
}

type T struct{}

func (*T) method() {
	_ = struct { // want `struct has size 24`
		a uint32
		b uint64
		c uint32
	}{}
}

func fn(param struct { // want `struct has size 24`
	a uint32
	b uint64
	c uint32
}) {
}
