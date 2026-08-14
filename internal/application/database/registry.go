package database

import (
	"sort"

	"github.com/dbvault/dbvault/internal/domain"
	"github.com/dbvault/dbvault/internal/ports"
)

type Registry struct {
	drivers map[domain.DatabaseEngine]ports.DatabaseDriver
}

func NewRegistry(drivers ...ports.DatabaseDriver) *Registry {
	r := &Registry{drivers: map[domain.DatabaseEngine]ports.DatabaseDriver{}}
	for _, d := range drivers {
		if d == nil {
			continue
		}
		r.drivers[d.Descriptor().Engine] = d
	}
	return r
}

func (r *Registry) Get(engine domain.DatabaseEngine) (ports.DatabaseDriver, bool) {
	d, ok := r.drivers[engine]
	return d, ok
}

func (r *Registry) List() []domain.DriverDescriptor {
	out := make([]domain.DriverDescriptor, 0, len(r.drivers))
	for _, d := range r.drivers {
		out = append(out, d.Descriptor())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Engine < out[j].Engine })
	return out
}
