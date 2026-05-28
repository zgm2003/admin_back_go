package enum

const (
	LoginTypeEmail    = "email"
	LoginTypePhone    = "phone"
	LoginTypePassword = "password"
)

var LoginTypes = []string{
	LoginTypeEmail,
	LoginTypePhone,
	LoginTypePassword,
}

func IsLoginType(value string) bool {
	for _, item := range LoginTypes {
		if value == item {
			return true
		}
	}
	return false
}
