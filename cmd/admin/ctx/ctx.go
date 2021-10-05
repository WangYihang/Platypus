package ctx

type Context struct {
	Token string
	Host  string
	Port  uint16
}

var Ctx Context

func IsValidToken(token string) bool {
	return token != ""
}

func GetHistoryFilepath() string {
	return ".history"
}
