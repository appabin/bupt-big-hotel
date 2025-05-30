# BUPT酒店管理系统

基于Go语言开发的酒店管理系统，使用Gin框架、GORM ORM、SQLite数据库和JWT认证。

## 功能特性

- 用户注册和登录（客户/管理员）
- JWT Token认证
- 房间管理（查看空房间、订房、退房）
- 空调控制（开关、温度、风速调节）
- 权限管理（客户只能操作自己的房间，管理员拥有全部权限）

## 技术栈

- **后端框架**: Gin
- **ORM**: GORM
- **数据库**: SQLite
- **认证**: JWT
- **密码加密**: bcrypt

## 项目结构

```
bupt-hotel/
├── config.go              # 配置文件
├── main.go                 # 主程序入口
├── go.mod                  # Go模块依赖
├── models/                 # 数据模型
│   ├── user.go            # 用户模型
│   └── room.go            # 房间和空调模型
├── database/               # 数据库相关
│   └── database.go        # 数据库初始化
├── middleware/             # 中间件
│   └── auth.go            # JWT认证中间件
└── handlers/               # API处理器
    ├── user.go            # 用户相关API
    ├── room.go            # 房间相关API
    └── airconditioner.go  # 空调相关API
```

## 快速开始

### 1. 安装依赖

```bash
go mod tidy
```

### 2. 运行项目

```bash
go run .
```

服务器将在 `http://localhost:8099` 启动

### 3. 健康检查

```bash
curl http://localhost:8099/health
```

## API文档

### 基础URL

```
http://localhost:8099/api
```

### 公开接口（无需认证）

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

### 需要认证的接口

所有需要认证的接口都需要在请求头中包含JWT Token:

```http
Authorization: Bearer <your-jwt-token>
```

#### 房间管理

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
DELETE /api/auth/rooms/101/checkout
Authorization: Bearer <token>
```

#### 空调控制

##### 获取空调信息

```http
GET /api/auth/airconditioner/101
Authorization: Bearer <token>
```

##### 控制空调

```http
PUT /api/auth/airconditioner/101
Authorization: Bearer <token>
Content-Type: application/json

{
  "ac_state": 1,           // 0: 关闭, 1: 开启
  "mode": "cooling",       // cooling 或 heating
  "current_speed": "high", // low, medium, high
  "target_temp": 22.5      // 目标温度 (16-30度)
}
```

### 管理员接口

#### 获取所有房间

```http
GET /api/admin/rooms
Authorization: Bearer <admin-token>
```

#### 获取所有空调信息

```http
GET /api/admin/airconditioners
Authorization: Bearer <admin-token>
```

## 默认数据

系统启动时会自动创建:

- **房间**: 101-110号房间（共10个房间）
- **管理员账户**: 
  - 用户名: `admin`
  - 密码: `password`
  - 身份: `administrator`

## 配置说明

可以通过环境变量配置系统参数:

- `JWT_SECRET`: JWT密钥（默认: bupt-hotel-secret-key-2024）
- `DATABASE_PATH`: 数据库文件路径（默认: ./hotel.db）
- `SERVER_PORT`: 服务器端口（默认: :8099）

## 数据模型

### 用户表 (User)

- `ID`: 用户ID（主键）
- `Username`: 用户名（唯一）
- `Password`: 密码（加密存储）
- `Identity`: 身份（customer/administrator）

### 房间信息表 (RoomInfo)

- `RoomID`: 房间号（主键）
- `ClientID`: 客户ID
- `ClientName`: 客户姓名
- `CheckinTime`: 入住时间
- `CheckoutTime`: 退房时间
- `State`: 房间状态（0: 空房, 1: 已入住）
- `DailyRate`: 每日房费
- `Deposit`: 押金

### 空调信息表 (AirConditioner)

- `ID`: 空调ID（主键）
- `RoomID`: 关联房间ID
- `ACState`: 空调状态（0: 关闭, 1: 开启）
- `Mode`: 模式（cooling/heating）
- `CurrentSpeed`: 当前风速
- `CurrentTemp`: 当前温度
- `TargetTemp`: 目标温度
- `InitialTemp`: 初始温度
- `LastPowerOnTime`: 最后开机时间
- `SwitchCount`: 开关次数

## 开发说明

1. 项目使用Go 1.21+
2. 数据库文件会自动创建在项目根目录
3. JWT Token有效期为24小时
4. 密码使用bcrypt加密
5. 支持CORS跨域请求

## 许可证

MIT License