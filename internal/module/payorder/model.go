package payorder

import "time"

type Order struct {
	ID                   int64      `gorm:"column:id;primaryKey"`
	OrderNo              string     `gorm:"column:order_no"`
	UserID               int64      `gorm:"column:user_id"`
	OrderType            int        `gorm:"column:order_type"`
	BizType              string     `gorm:"column:biz_type"`
	BizID                int64      `gorm:"column:biz_id"`
	Title                string     `gorm:"column:title"`
	ItemCount            int        `gorm:"column:item_count"`
	TotalAmount          int        `gorm:"column:total_amount"`
	DiscountAmount       int        `gorm:"column:discount_amount"`
	PayAmount            int        `gorm:"column:pay_amount"`
	PayStatus            int        `gorm:"column:pay_status"`
	BizStatus            int        `gorm:"column:biz_status"`
	SuccessTransactionID int64      `gorm:"column:success_transaction_id"`
	ChannelID            int64      `gorm:"column:channel_id"`
	PayMethod            string     `gorm:"column:pay_method"`
	PayTime              *time.Time `gorm:"column:pay_time"`
	ExpireTime           time.Time  `gorm:"column:expire_time"`
	CloseTime            *time.Time `gorm:"column:close_time"`
	BizDoneAt            *time.Time `gorm:"column:biz_done_at"`
	CloseReason          string     `gorm:"column:close_reason"`
	FailReason           string     `gorm:"column:fail_reason"`
	Extra                string     `gorm:"column:extra"`
	AdminRemark          string     `gorm:"column:admin_remark"`
	IP                   string     `gorm:"column:ip"`
	IsDel                int        `gorm:"column:is_del"`
	CreatedAt            time.Time  `gorm:"column:created_at"`
	UpdatedAt            time.Time  `gorm:"column:updated_at"`
}

func (Order) TableName() string { return "orders" }

type OrderItem struct {
	ID        int64     `gorm:"column:id;primaryKey"`
	OrderID   int64     `gorm:"column:order_id"`
	ItemType  string    `gorm:"column:item_type"`
	Title     string    `gorm:"column:title"`
	Price     int       `gorm:"column:price"`
	Quantity  int       `gorm:"column:quantity"`
	Amount    int       `gorm:"column:amount"`
	IsDel     int       `gorm:"column:is_del"`
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

func (OrderItem) TableName() string { return "order_items" }
