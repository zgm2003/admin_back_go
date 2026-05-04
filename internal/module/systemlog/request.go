package systemlog

type linesRequest struct {
	Tail    int    `form:"tail" binding:"omitempty,min=1,max=2000"`
	Level   string `form:"level" binding:"omitempty,log_level"`
	Keyword string `form:"keyword" binding:"omitempty,max=200"`
}
