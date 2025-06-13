# BUPT酒店管理系统

基于Go语言开发的智能酒店管理系统，集成了先进的空调调度算法、实时状态监控和完整的酒店业务流程管理。系统采用Gin框架、GORM ORM、SQLite数据库和JWT认证，提供高性能的RESTful API服务。

## 🌟 核心特性

### 用户管理
- 多角色用户系统（客户/管理员）
- JWT Token安全认证
- bcrypt密码加密存储
- 细粒度权限控制

### 房间管理
- 多房间类型支持
- 实时房间状态管理
- 在线预订和退房系统
- 房间账单和费用计算
- Excel报表生成

### 智能空调调度系统
- **三队列调度算法**：服务队列、缓冲队列、回温队列
- **优先级调度**：高/中/低三级优先级管理
- **时间片轮转**：公平的资源分配机制
- **实时温度控制**：精确的温度调节和监控
- **长轮询状态更新**：实时状态同步
- **详细操作记录**：完整的空调使用历史

### 数据管理
- 完整的数据模型设计
- 自动数据库迁移
- 详细的操作日志记录
- 数据报表导出功能

## 🛠 技术栈

- **后端框架**: Gin v1.9.1
- **ORM**: GORM v1.25.5
- **数据库**: SQLite 3
- **认证**: JWT v5.0.0
- **密码加密**: bcrypt
- **Excel处理**: excelize v2.9.1
- **Go版本**: 1.24+

## 📁 项目结构

```
bupt-hotel/
├── config.go                  # 系统配置管理
├── main.go                     # 主程序入口和路由配置
├── go.mod                      # Go模块依赖管理
├── go.sum                      # 依赖版本锁定
├── models/                     # 数据模型层
│   ├── user.go                # 用户数据模型
│   ├── room.go                # 房间相关数据模型
│   ├── room_type_data.go      # 房间类型数据
│   ├── airconditioner.go      # 空调数据模型
│   └── scheduler.go           # 调度器数据模型
├── database/                   # 数据库层
│   └── database.go            # 数据库初始化和配置
├── middleware/                 # 中间件层
│   └── auth.go                # JWT认证和权限中间件
└── handlers/                   # 业务逻辑处理层
    ├── user.go                # 用户管理API
    ├── room.go                # 房间管理API
    ├── ac_operations.go       # 空调操作API
    ├── scheduler.go           # 空调调度器核心逻辑
    └── admin.go               # 管理员功能API
```

## 🚀 快速开始

### 环境要求
- Go 1.24+
- SQLite 3

### 1. 克隆项目

```bash
git clone <repository-url>
cd bupt-hotel
```

### 2. 安装依赖

```bash
go mod tidy
```

### 3. 配置环境变量（可选）

```bash
export JWT_SECRET="your-secret-key"
export DATABASE_PATH="./hotel.db"
export SERVER_PORT=":8099"
```

### 4. 运行项目

```bash
go run .
```

服务器将在 `http://localhost:8099` 启动

### 5. 健康检查

```bash
curl http://localhost:8099/health
```

响应:
```json
{
  "status": "ok",
  "message": "BUPT酒店管理系统运行正常"
}
```

## 📚 API文档

### 基础URL

```
http://localhost:8099/api
```

### 🔓 公开接口（无需认证）

#### 用户注册

```http
POST /api/public/register
Content-Type: application/json

{
  "username": "testuser",
  "password": "password123",
  "identity": "customer"  // customer 或 administrator
}
```

#### 用户登录

```http
POST /api/public/login
Content-Type: application/json

{
  "username": "testuser",
  "password": "password123"
}
```

响应:
```json
{
  "message": "登录成功",
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "user": {
    "id": 1,
    "username": "testuser",
    "identity": "customer"
  }
}
```

### 🔐 需要认证的接口

所有需要认证的接口都需要在请求头中包含JWT Token:

```http
Authorization: Bearer <your-jwt-token>
```

#### 房间管理

##### 获取房间类型

```http
GET /api/auth/rooms/type
Authorization: Bearer <token>
```

##### 按类型获取房间

```http
GET /api/auth/rooms/by-type/:type_id
Authorization: Bearer <token>
```

##### 获取空房间

```http
GET /api/auth/rooms/available
Authorization: Bearer <token>
```

##### 获取我的房间

```http
GET /api/auth/rooms/my
Authorization: Bearer <token>
```

##### 订房

```http
POST /api/auth/rooms/book
Authorization: Bearer <token>
Content-Type: application/json

{
  "room_id": 101,
  "client_name": "张三",
  "days": 3
}
```

##### 退房

```http
POST /api/auth/rooms/:room_id/checkout
Authorization: Bearer <token>
```

#### 空调控制

##### 控制空调

```http
PUT /api/auth/airconditioner/:room_id
Authorization: Bearer <token>
Content-Type: application/json

{
  "operation_type": 0,     // 0: 开机, 1: 关机, 2: 调温
  "mode": "cooling",       // cooling 或 heating
  "speed": "high",         // low, medium, high
  "target_temp": 225       // 目标温度*10 (160-300)
}
```

##### 长轮询获取空调状态

```http
GET /api/auth/airconditioner/:room_id/status
Authorization: Bearer <token>
```

响应:
```json
{
  "room_id": 101,
  "ac_status": 0,          // 0: 运行, 1: 暂停服务, 2: 停机
  "current_temp": 245,     // 当前温度*10
  "target_temp": 225,      // 目标温度*10
  "current_speed": "high", // 当前风速
  "mode": "cooling",       // 当前模式
  "current_cost": 12.5,    // 当前费用
  "total_cost": 25.0,      // 总费用
  "running_time": 3600,    // 运行时间(秒)
  "priority": 1            // 优先级 1:高 2:中 3:低
}
```

### 👨‍💼 管理员接口

#### 获取所有房间

```http
GET /api/admin/rooms
Authorization: Bearer <admin-token>
```

#### 获取调度器状态

```http
GET /api/admin/scheduler/status
Authorization: Bearer <admin-token>
```

#### 获取详细调度器状态

```http
GET /api/admin/scheduler
Authorization: Bearer <admin-token>
```

响应:
```json
{
  "serving_queue": [
    {
      "acid": 1,
      "room_id": 101,
      "ac_state": 0,
      "mode": "cooling",
      "priority": 1,
      "current_temp": 245,
      "target_temp": 225,
      "current_speed": "high"
    }
  ],
  "buffer_queue": [...],
  "warming_queue": [...],
  "tick_count": 1250,
  "is_running": true
}
```

#### 更新房间类型

```http
PUT /api/admin/room-types/:id
Authorization: Bearer <admin-token>
Content-Type: application/json

{
  "type": "豪华套房",
  "description": "高端豪华套房",
  "price_range": "800-1200元/晚",
  "features": ["海景", "按摩浴缸", "私人阳台"]
}
```

## 🗄️ 默认数据

系统启动时会自动创建:

### 房间数据
- **房间**: 101-110号房间（共10个房间）
- **房间类型**: 标准间、豪华间、套房等多种类型
- **空调设备**: 每个房间配备独立空调系统

### 默认账户
- **管理员账户**: 
  - 用户名: `admin`
  - 密码: `password`
  - 身份: `administrator`

## ⚙️ 配置说明

可以通过环境变量配置系统参数:

- `JWT_SECRET`: JWT密钥（默认: bupt-hotel-secret-key-2025）
- `DATABASE_PATH`: 数据库文件路径（默认: ./hotel.db）
- `SERVER_PORT`: 服务器端口（默认: :8099）

### 空调调度器配置
- **服务队列容量**: 最多3台空调同时服务
- **时间片大小**: 2个tick（约2秒）
- **调度周期**: 每10个tick进行一次队列重排
- **温度精度**: 0.1°C（存储时*10）
- **优先级策略**: 高优先级优先服务

## 🗃️ 数据模型

### 用户表 (User)

- `ID`: 用户ID（主键）
- `Username`: 用户名（唯一）
- `Password`: 密码（bcrypt加密存储）
- `Identity`: 身份（customer/administrator）

### 房间类型表 (RoomType)

- `ID`: 房间类型ID（主键）
- `Type`: 房间类型名称
- `Description`: 房间描述
- `PriceRange`: 价格范围
- `Features`: 房间特色功能列表
- `CreatedAt`: 创建时间
- `UpdatedAt`: 更新时间

### 房间信息表 (RoomInfo)

- `RoomID`: 房间号（主键）
- `RoomTypeID`: 关联房间类型ID
- `ClientID`: 客户ID
- `ClientName`: 客户姓名
- `CheckinTime`: 入住时间
- `CheckoutTime`: 退房时间
- `State`: 房间状态（0: 空房, 1: 已入住）
- `DailyRate`: 每日房费
- `Deposit`: 押金

### 房间操作记录表 (RoomOperation)

- `ID`: 记录ID（主键）
- `RoomID`: 房间ID
- `BillID`: 账单号
- `ClientID`: 客户ID
- `ClientName`: 客户姓名
- `OperationType`: 操作类型（checkin/checkout）
- `OperationTime`: 操作时间
- `CheckinTime`: 入住时间
- `CheckoutTime`: 退房时间
- `DailyRate`: 每日房费
- `Deposit`: 押金
- `TotalCost`: 总费用
- `ActualDays`: 实际入住天数

### 空调信息表 (AirConditioner)

- `ID`: 空调ID（主键）
- `RoomID`: 关联房间ID
- `EnvironmentTemp`: 环境温度*10

### 空调操作记录表 (AirConditionerOperation)

- `ID`: 记录ID（主键）
- `BillID`: 账单号
- `RoomID`: 房间ID
- `AcID`: 空调ID
- `OperationState`: 操作状态（0: 开机, 1: 关机, 2: 调温）
- `Speed`: 风速（high/medium/low）
- `Mode`: 模式（cooling/heating）
- `CurrentTemp`: 当前温度*10
- `TargetTemp`: 目标温度*10
- `EnvironmentTemp`: 环境温度*10
- `CurrentCost`: 当前费用
- `TotalCost`: 总费用
- `RunningTime`: 运行时间（秒）
- `Priority`: 优先级（1: 高, 2: 中, 3: 低）
- `CreatedAt`: 创建时间

### 空调详细记录表 (AirConditionerDetail)

- `ID`: 记录ID（主键）
- `BillID`: 账单号
- `RoomID`: 房间ID
- `AcID`: 空调ID
- `CurrentTemp`: 当前温度*10
- `TargetTemp`: 目标温度*10
- `Speed`: 风速
- `Mode`: 模式
- `CurrentCost`: 当前费用
- `TotalCost`: 总费用
- `RunningTime`: 运行时间
- `ACState`: 空调状态
- `CreatedAt`: 记录时间

### 调度器模型 (Scheduler)

- `ACID`: 空调ID
- `BillID`: 账单号
- `RoomID`: 房间ID
- `ACState`: 空调状态（0: 运行, 1: 等待, 2: 关机回温, 3: 达到目标温度回温）
- `Mode`: 模式（cooling/heating）
- `Priority`: 优先级（1: 高, 2: 中, 3: 低）
- `CurrentSpeed`: 当前风速
- `CurrentTemp`: 当前温度*10
- `TargetTemp`: 目标温度*10
- `EnvironmentTemp`: 环境温度*10
- `RoundRobinCount`: 时间片计数
- `ServiceTime`: 服务时间
- `TotalCost`: 总费用
- `RunningTime`: 运行时间

## 🔧 开发说明

### 基本要求
1. 项目使用Go 1.24+
2. 数据库文件会自动创建在项目根目录
3. JWT Token有效期为24小时
4. 密码使用bcrypt加密
5. 支持CORS跨域请求

### 空调调度算法

#### 三队列架构
- **服务队列 (Serving Queue)**: 最多容纳3台正在运行的空调
- **缓冲队列 (Buffer Queue)**: 等待服务的空调队列
- **回温队列 (Warming Queue)**: 关机或达到目标温度的空调回温队列

#### 调度策略
1. **优先级调度**: 高优先级(1) > 中优先级(2) > 低优先级(3)
2. **时间片轮转**: 每个空调在服务队列中最多服务2个时间片
3. **公平调度**: 同优先级内按服务时间和ID排序
4. **动态调整**: 每10个tick重新排序缓冲队列

#### 温度控制
- 制冷模式：温度每tick下降1°C（直到目标温度）
- 制热模式：温度每tick上升1°C（直到目标温度）
- 回温模式：每2个tick变化1°C（趋向环境温度）

#### 费用计算
- 高风速：1元/分钟
- 中风速：0.5元/分钟
- 低风速：0.33元/分钟

### 开发特性
- 实时日志记录
- 长轮询状态更新
- Excel报表生成
- 完整的错误处理
- 并发安全设计

## 📄 许可证

MIT License

## 🤝 贡献

欢迎提交Issue和Pull Request来改进项目。

## 📞 联系方式

如有问题，请通过以下方式联系：
- 项目Issues: [GitHub Issues]()
- 邮箱: [your-email@example.com]()

---

**BUPT酒店管理系统** - 智能、高效、可靠的酒店管理解决方案