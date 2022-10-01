package Controller

type RoleList struct {
	Get  bool   `json:"get"`
	Role string `json:"role"`
}

type ServerAccess struct {
	Get  bool   `json:"get"`
	Info string `json:"info"`
	Hash string `json:"hash"`
}
