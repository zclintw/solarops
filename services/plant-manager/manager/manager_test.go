package manager

import (
	"testing"
)

func TestPlantRegistryAddAndList(t *testing.T) {
	r := NewRegistry()

	r.Add("id-1", "Sunrise Valley", 50, 300, "container-1")
	r.Add("id-2", "Golden Ridge", 30, 250, "container-2")

	plants := r.List()
	if len(plants) != 2 {
		t.Errorf("expected 2 plants, got %d", len(plants))
	}
}

func TestPlantRegistryNameExists(t *testing.T) {
	r := NewRegistry()
	r.Add("id-1", "Sunrise Valley", 50, 300, "container-1")

	if !r.NameExists("Sunrise Valley") {
		t.Error("expected name to exist")
	}
	if r.NameExists("Golden Ridge") {
		t.Error("expected name to not exist")
	}
}

func TestPlantRegistryRemove(t *testing.T) {
	r := NewRegistry()
	r.Add("id-1", "Sunrise Valley", 50, 300, "container-1")

	info, ok := r.Remove("id-1")
	if !ok {
		t.Error("expected to find plant")
	}
	if info.ContainerID != "container-1" {
		t.Errorf("expected container-1, got %s", info.ContainerID)
	}

	plants := r.List()
	if len(plants) != 0 {
		t.Errorf("expected 0 plants after remove, got %d", len(plants))
	}
}
