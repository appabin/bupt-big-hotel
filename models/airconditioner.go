package models

import "time"

// 空调信息表
type AirConditioner struct {
	ID              int `gorm:"primaryKey"`
	RoomID          int `gorm:"type:int;index"`       // 关联房间ID
	EnvironmentTemp int `gorm:"type:int;default:250"` // 环境温度*10
}

// 空调操作表
type AirConditionerOperation struct {
	ID     int `gorm:"primaryKey"`
	BillID int `gorm:"type:int;index"` // 订单号
	RoomID int `gorm:"type:int;index"` // 房间ID
	AcID   int `gorm:"type:int;index"` // 关联空调ID

	// 空调操作状态：0-开机 1-关机 2-调温
	OperationState int `gorm:"type:int;default:1"` // 0: 开机 1: 关机 2: 调温

	// 风速：high-高速 medium-中速 low-低速
	Speed string `gorm:"type:varchar(20);default:'medium'"` // high/medium/low

	// 模式：cooling-制冷 heating-制热
	Mode string `gorm:"type:varchar(20);default:'cooling'"` // cooling/heating

	// 温度信息（*10存储，如245表示24.5度）
	TargetTemp      int `gorm:"type:int;default:250"` // 目标温度*10
	EnvironmentTemp int `gorm:"type:int;default:250"` // 环境温度*10
	CurrentTemp     int `gorm:"type:int;default:250"` // 当前温度*10

	// 费用信息
	CurrentCost float32 `gorm:"type:float(10,2);default:0"` // 当前花费金额
	TotalCost   float32 `gorm:"type:float(10,2);default:0"` // 总花费金额

	// 时间信息
	RunningTime        int `gorm:"type:int"` // 记录运行时间
	CurrentRunningTime int `gorm:"type:int"` // 记录总运行时间

	// 统计信息
	SwitchCount int       `gorm:"type:int;default:0"` // 开关次数
	CreatedAt   time.Time `gorm:"autoCreateTime"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime"`
}

// 空调状态表
type AirConditionerDetail struct {
	ID     int `gorm:"primary_key"`
	BillID int `gorm:"type:int;index"` // 订单号
	RoomID int `gorm:"type:int;index"` // 房间ID
	AcID   int `gorm:"type:int;index"` // 关联空调ID

	ACStatus int `gorm:"type:int"` //0-运行 1-在等待序列 2-关机回温 3-达到目标温度回温

	// 风速：high-高速 medium-中速 low-低速
	Speed string `gorm:"type:varchar(20)"` // high/medium/low

	// 模式：cooling-制冷 heating-制热
	Mode string `gorm:"type:varchar(20)"` // cooling/heating

	// 温度信息（*10存储，如245表示24.5度）
	TargetTemp      int `gorm:"type:int"` // 目标温度*10
	EnvironmentTemp int `gorm:"type:int"` // 环境温度*10
	CurrentTemp     int `gorm:"type:int"` // 当前温度*10

	RunningTime        int `gorm:"type:int"` // 记录运行时间
	CurrentRunningTime int `gorm:"type:int"` // 记录总运行时间
	// 费用信息
	CurrentCost float32 `gorm:"type:float(10,2)"` // 当前花费金额
	TotalCost   float32 `gorm:"type:float(10,2)"` // 总花费金额

	// 费率和变化信息
	Rate       float32 `gorm:"type:float(5,2)"` // 每分钟费率(元/分钟)
	TempChange int     `gorm:"type:int"`        // 温度变化*10

	// 记录时间
	CreatedAt time.Time `gorm:"autoCreateTime"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
}
