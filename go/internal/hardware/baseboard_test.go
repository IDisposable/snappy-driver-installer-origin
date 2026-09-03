package hardware

import "testing"

func TestLastChassisType(t *testing.T) {
	if got := lastChassisType(nil); got != 0 {
		t.Errorf("lastChassisType(nil) = %d, want 0", got)
	}
	if got := lastChassisType([]int{3, 9, 1}); got != 1 {
		t.Errorf("lastChassisType([3,9,1]) = %d, want 1 (last element)", got)
	}
	if got := lastChassisType([]int{10}); got != 10 {
		t.Errorf("lastChassisType([10]) = %d, want 10", got)
	}
}
