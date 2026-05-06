package payruntime

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

type PayTransaction struct {
	ID            int64      `gorm:"column:id;primaryKey"`
	TransactionNo string     `gorm:"column:transaction_no"`
	OrderID       int64      `gorm:"column:order_id"`
	OrderNo       string     `gorm:"column:order_no"`
	AttemptNo     int        `gorm:"column:attempt_no"`
	ChannelID     int64      `gorm:"column:channel_id"`
	Channel       int        `gorm:"column:channel"`
	PayMethod     string     `gorm:"column:pay_method"`
	Amount        int        `gorm:"column:amount"`
	TradeNo       string     `gorm:"column:trade_no"`
	TradeStatus   string     `gorm:"column:trade_status"`
	Status        int        `gorm:"column:status"`
	PaidAt        *time.Time `gorm:"column:paid_at"`
	ClosedAt      *time.Time `gorm:"column:closed_at"`
	ChannelResp   string     `gorm:"column:channel_resp"`
	RawNotify     string     `gorm:"column:raw_notify"`
	IsDel         int        `gorm:"column:is_del"`
	CreatedAt     time.Time  `gorm:"column:created_at"`
	UpdatedAt     time.Time  `gorm:"column:updated_at"`
}

func (PayTransaction) TableName() string { return "pay_transactions" }

type Channel struct {
	ID                int64     `gorm:"column:id;primaryKey"`
	Name              string    `gorm:"column:name"`
	Channel           int       `gorm:"column:channel"`
	MchID             string    `gorm:"column:mch_id"`
	AppID             string    `gorm:"column:app_id"`
	NotifyURL         string    `gorm:"column:notify_url"`
	AppPrivateKeyEnc  string    `gorm:"column:app_private_key_enc"`
	AppPrivateKeyHint string    `gorm:"column:app_private_key_hint"`
	PublicCertPath    string    `gorm:"column:public_cert_path"`
	PlatformCertPath  string    `gorm:"column:platform_cert_path"`
	RootCertPath      string    `gorm:"column:root_cert_path"`
	ExtraConfig       string    `gorm:"column:extra_config"`
	IsSandbox         int       `gorm:"column:is_sandbox"`
	Sort              int       `gorm:"column:sort"`
	Remark            string    `gorm:"column:remark"`
	Status            int       `gorm:"column:status"`
	IsDel             int       `gorm:"column:is_del"`
	CreatedAt         time.Time `gorm:"column:created_at"`
	UpdatedAt         time.Time `gorm:"column:updated_at"`
}

func (Channel) TableName() string { return "pay_channel" }

type PayNotifyLog struct {
	ID             int64     `gorm:"column:id;primaryKey"`
	Channel        int       `gorm:"column:channel"`
	NotifyType     int       `gorm:"column:notify_type"`
	TransactionNo  string    `gorm:"column:transaction_no"`
	TradeNo        string    `gorm:"column:trade_no"`
	Headers        string    `gorm:"column:headers"`
	RawData        string    `gorm:"column:raw_data"`
	ProcessStatus  int       `gorm:"column:process_status"`
	ProcessMessage string    `gorm:"column:process_msg"`
	IP             string    `gorm:"column:ip"`
	IsDel          int       `gorm:"column:is_del"`
	CreatedAt      time.Time `gorm:"column:created_at"`
	UpdatedAt      time.Time `gorm:"column:updated_at"`
}

func (PayNotifyLog) TableName() string { return "pay_notify_logs" }

type OrderFulfillment struct {
	ID             int64      `gorm:"column:id;primaryKey"`
	FulfillNo      string     `gorm:"column:fulfill_no"`
	OrderID        int64      `gorm:"column:order_id"`
	OrderNo        string     `gorm:"column:order_no"`
	UserID         int64      `gorm:"column:user_id"`
	BizType        string     `gorm:"column:biz_type"`
	BizID          int64      `gorm:"column:biz_id"`
	ActionType     int        `gorm:"column:action_type"`
	SourceTxnID    int64      `gorm:"column:source_txn_id"`
	IdempotencyKey string     `gorm:"column:idempotency_key"`
	Status         int        `gorm:"column:status"`
	RetryCount     int        `gorm:"column:retry_count"`
	NextRetryAt    *time.Time `gorm:"column:next_retry_at"`
	ExecutedAt     *time.Time `gorm:"column:executed_at"`
	LastError      string     `gorm:"column:last_error"`
	RequestPayload string     `gorm:"column:request_payload"`
	ResultPayload  string     `gorm:"column:result_payload"`
	IsDel          int        `gorm:"column:is_del"`
	CreatedAt      time.Time  `gorm:"column:created_at"`
	UpdatedAt      time.Time  `gorm:"column:updated_at"`
}

func (OrderFulfillment) TableName() string { return "order_fulfillments" }

type UserWallet struct {
	ID            int64     `gorm:"column:id;primaryKey"`
	UserID        int64     `gorm:"column:user_id"`
	Balance       int       `gorm:"column:balance"`
	Frozen        int       `gorm:"column:frozen"`
	TotalRecharge int       `gorm:"column:total_recharge"`
	TotalConsume  int       `gorm:"column:total_consume"`
	Version       int       `gorm:"column:version"`
	IsDel         int       `gorm:"column:is_del"`
	CreatedAt     time.Time `gorm:"column:created_at"`
	UpdatedAt     time.Time `gorm:"column:updated_at"`
}

func (UserWallet) TableName() string { return "user_wallets" }

type WalletTransaction struct {
	ID             int64     `gorm:"column:id;primaryKey"`
	BizActionNo    string    `gorm:"column:biz_action_no"`
	UserID         int64     `gorm:"column:user_id"`
	WalletID       int64     `gorm:"column:wallet_id"`
	Type           int       `gorm:"column:type"`
	AvailableDelta int       `gorm:"column:available_delta"`
	FrozenDelta    int       `gorm:"column:frozen_delta"`
	BalanceBefore  int       `gorm:"column:balance_before"`
	BalanceAfter   int       `gorm:"column:balance_after"`
	FrozenBefore   int       `gorm:"column:frozen_before"`
	FrozenAfter    int       `gorm:"column:frozen_after"`
	OrderID        int64     `gorm:"column:order_id"`
	OrderNo        string    `gorm:"column:order_no"`
	SourceType     int       `gorm:"column:source_type"`
	SourceID       int64     `gorm:"column:source_id"`
	Title          string    `gorm:"column:title"`
	Remark         string    `gorm:"column:remark"`
	OperatorID     int64     `gorm:"column:operator_id"`
	Ext            string    `gorm:"column:ext"`
	IsDel          int       `gorm:"column:is_del"`
	CreatedAt      time.Time `gorm:"column:created_at"`
	UpdatedAt      time.Time `gorm:"column:updated_at"`
}

func (WalletTransaction) TableName() string { return "wallet_transactions" }

type User struct {
	ID    int64 `gorm:"column:id;primaryKey"`
	IsDel int   `gorm:"column:is_del"`
}

func (User) TableName() string { return "users" }
