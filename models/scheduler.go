package models

type Scheduler struct {
	ACID               int
	BillID             int
	RoomID             int
	ACState            int //0-运行 1-在等待序列 2-关机回温 3-达到目标温度回温
	Mode               string
	Priority           int // 1: high, 2: medium, 3: low
	CurrentSpeed       string
	CurrentTemp        int
	TargetTemp         int
	EnvironmentTemp    int
	CurrentCost        int
	TotalCost          int
	RunningTime        int
	CurrentRunningTime int
	RoundRobinCount    int
}
