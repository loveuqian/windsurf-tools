package diagnose

import "testing"

func TestIntToStr(t *testing.T) {
	cases := map[int]string{
		0: "0", 1: "1", 9: "9", 10: "10", 100: "100",
		42: "42", -7: "-7", -100: "-100",
	}
	for n, want := range cases {
		if got := intToStr(n); got != want {
			t.Errorf("intToStr(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestWindsurfCandidatesByOS_NotEmpty(t *testing.T) {
	cands := windsurfCandidatesByOS()
	if len(cands) == 0 {
		t.Error("应返回至少 1 个候选路径（即使 OS 不识别）")
	}
}
