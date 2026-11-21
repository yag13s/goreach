package sample

import (
	"context"
	"strings"

	yaml "gopkg.in/yaml.v3"
)

type store struct{}

func (s *store) Get(ctx context.Context, id string) string { return id }

func lookup(id string) string { return id }

func loops(ctx context.Context, s *store, ids []string) {
	s.Get(ctx, "before")
	for _, id := range lookupAll(ids) {
		s.Get(ctx, id)
		_ = len(id)
		_ = strings.ToUpper(id)
		go func() { lookup(id) }()
	}
	for i := 0; i < count(); i++ {
		_ = yaml.Marshal
	}
	for i := setup(); i < 3; next(&i) {
	}
}

func lookupAll(ids []string) []string { return ids }
func count() int                      { return 0 }
func setup() int                      { return 0 }
func next(i *int)                     { *i++ }
