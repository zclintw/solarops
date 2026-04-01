package manager

import (
	"testing"
)

func TestPlantRegistryAddAndList(t *testing.T) {
	r := NewRegistry()

	r.Add("id-1", "Sunrise Valley", 50, 300)
	r.Add("id-2", "Golden Ridge", 30, 250)

	plants := r.List()
	if len(plants) != 2 {
		t.Errorf("expected 2 plants, got %d", len(plants))
	}
}

func TestPlantRegistryGet(t *testing.T) {
	r := NewRegistry()
	r.Add("id-1", "Sunrise Valley", 50, 300)

	info, ok := r.Get("id-1")
	if !ok {
		t.Error("expected to find plant")
	}
	if info.PlantName != "Sunrise Valley" {
		t.Errorf("expected Sunrise Valley, got %s", info.PlantName)
	}

	_, ok = r.Get("id-999")
	if ok {
		t.Error("expected not to find plant")
	}
}
