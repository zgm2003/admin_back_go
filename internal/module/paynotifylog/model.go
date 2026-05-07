package paynotifylog

import "time"

type PayNotifyLog struct {
	ID            int64     `gorm:"column:id;primaryKey"`
	Channel       int       `gorm:"column:channel"`
	NotifyType    int       `gorm:"column:notify_type"`
	TransactionNo string    `gorm:"column:transaction_no"`
	TradeNo       string    `gorm:"column:trade_no"`
	Headers       string    `gorm:"column:headers"`
	RawData       string    `gorm:"column:raw_data"`
	ProcessStatus int       `gorm:"column:process_status"`
	ProcessMsg    string    `gorm:"column:process_msg"`
	IP            string    `gorm:"column:ip"`
	IsDel         int       `gorm:"column:is_del"`
	CreatedAt     time.Time `gorm:"column:created_at"`
	UpdatedAt     time.Time `gorm:"column:updated_at"`
}

func (PayNotifyLog) TableName() string {
	return "pay_notify_logs"
}
