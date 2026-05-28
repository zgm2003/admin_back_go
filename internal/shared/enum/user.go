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

const (
	VerifyTypePassword = "password"
	VerifyTypeCode     = "code"
)

var UserVerifyTypes = []string{
	VerifyTypePassword,
	VerifyTypeCode,
}

func IsUserVerifyType(value string) bool {
	for _, item := range UserVerifyTypes {
		if value == item {
			return true
		}
	}
	return false
}
