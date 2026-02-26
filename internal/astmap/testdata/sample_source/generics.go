package sample

// Generic types to exercise IndexExpr and IndexListExpr AST nodes

type Container[T any] struct {
	Value T
}

func (c Container[T]) Get() T {
	return c.Value
}

func (c *Container[T]) Set(v T) {
	c.Value = v
}

type Pair[K comparable, V any] struct {
	Key   K
	Value V
}

func (p Pair[K, V]) GetKey() K {
	return p.Key
}

func (p *Pair[K, V]) SetKey(k K) {
	p.Key = k
}
