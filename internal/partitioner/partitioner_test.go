package partitioner

import "testing"

func TestOwnsPodUIDSingleShard(t *testing.T) {
	p := New(1, 0)
	if !p.OwnsPodUID("any-uid") {
		t.Fatalf("expected single shard to own all pods")
	}
}

func TestOwnsPodUIDExactlyOneOwner(t *testing.T) {
	total := 3
	uid := "uid-1234"
	owners := 0
	for i := 0; i < total; i++ {
		if New(total, i).OwnsPodUID(uid) {
			owners++
		}
	}
	if owners != 1 {
		t.Fatalf("expected exactly one owner, got %d", owners)
	}
}
