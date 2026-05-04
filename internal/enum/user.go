package enum

const (
	SexUnknown = 0
	SexMale    = 1
	SexFemale  = 2
)

var Sexes = []int{
	SexUnknown,
	SexMale,
	SexFemale,
}

func IsSex(value int) bool {
	for _, item := range Sexes {
		if value == item {
			return true
		}
	}
	return false
}
