package routes

import (
	"TaskManagement_System/middlewares"
	"TaskManagement_System/models"
	"TaskManagement_System/utils"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"gorm.io/gorm"
	"log"
	"net/http"
	"sync"
)

// 使用WaitGroup来等待所有goroutine完成
var Wg sync.WaitGroup

func SetupRouter(r *gin.Engine) {
	//分路由组

	authGroup := r.Group("/auth")
	{
		authGroup.POST("/register", RegisterHandler)
		authGroup.POST("/login", LoginHandler)
		authGroup.GET("/allusers", GetallUsers)
	}
	//认证加中间件
	taskGroup := r.Group("/tasks")
	taskGroup.Use(middlewares.JWTAuthMiddleware())
	{
		taskGroup.POST("/", CreateTask)
		taskGroup.GET("/", GetallTasks)
		taskGroup.GET("/:id", GetsingleTask)
		taskGroup.PUT("/:id", UpdateTask)
		taskGroup.DELETE("/:id", DeleteTask)
		taskGroup.POST("/import", ImportTask)
	}
}
func GetallUsers(c *gin.Context) { //仅供调试（查看所有用户）
	var users []models.User
	db := c.MustGet("db").(*gorm.DB)
	if err := db.Model(&models.User{}).Find(&users).Error; err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"users": users})
}
func CreateTask(c *gin.Context) { //创建任务
	//迁移通常不是直接在这里进行的。迁移通常是在应用启动或数据库初始化时执行的，而不是在每个API端点中单独执行。
	db := c.MustGet("db").(*gorm.DB)

	ownerID := viper.GetUint("OwnerID")
	task := models.Task{
		Title:       "tasktitle",
		Description: "taskdescription",
		Status:      -1, //pending 对应 -1   in-progress 对应 1   completed 对应0
		OwnerID:     ownerID,
		//创建时间，更新时间自动获取。
		//这里加入用户关联。关联用户ID  属于 一对多的关系  一个任务 对应 多个玩家
		//创建任务应该自己决定>
	}
	if err := c.ShouldBindJSON(&task); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	//加入错误判断
	//...
	result := db.Create(&task)
	if result.Error != nil {
		c.JSON(400, gin.H{"error": result.Error.Error()})
		return
	}
	c.JSON(201, task)
	//201 通常表示已创建（Created），这是当客户端的请求导致在服务器上创建了一个新资源时使用的状态码。
}

func UpdateTask(c *gin.Context) { //更新任务
	var task models.Task
	db := c.MustGet("db").(*gorm.DB)
	if err := c.ShouldBindJSON(&task); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	//处理更新任务的数据

	//让用户确认要更新的任务ID

	//这里做一个用户输入，考虑怎么做
	// 从URL参数中获取ID
	id := c.Param("id")
	if err := db.Model(&models.Task{}).Where("id = ?", id).Updates(task).Error; err != nil { //传的是结构体指针，可能省一点内存
		// //这里记得考虑结构体字段的问题，因为0值忽略。不然就换成多个update
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{"message": "task updated successfully"})

}

func DeleteTask(c *gin.Context) { //删除任务
	db := c.MustGet("db").(*gorm.DB)

	id := c.Param("id") //删除 直接通过id删除
	if err := db.Model(&models.Task{}).Where("id = ?", id).Delete(&models.Task{}).Error; err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{"message": "task deleted successfully"})
}

func GetallTasks(c *gin.Context) {
	var tasks []models.Task
	db := c.MustGet("db").(*gorm.DB)
	if err := db.Model(&models.Task{}).Find(&tasks).Error; err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"tasks": tasks})
} //获取所有任务  -》输出？

func GetsingleTask(c *gin.Context) {
	var task models.Task
	db := c.MustGet("db").(*gorm.DB)
	id := c.Param("id")
	if err := db.Model(&models.Task{}).Where("id = ?", id).First(&task).Error; err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return

	}
	c.JSON(200, gin.H{"message": task})

} //获取单个任务 -》 输出？

//批量的任务导入

func ImportTask(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	var taskBatch models.TaskBatch
	if err := c.ShouldBindJSON(&taskBatch); err != nil { //先绑定JSON数据到taskBatch上
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	//数组遍历
	for _, task := range taskBatch.Tasks {
		Wg.Add(1) //每个任务启动一个goroutine
		go func(t models.Task) {
			defer Wg.Done()
			if err := db.Create(&task).Error; err != nil {
				c.JSON(500, gin.H{"error": "Create Fail"})
			}
		}(task)

	}
	Wg.Wait()
	c.JSON(200, gin.H{"message": "Tasks imported successfully"})
	//接收一个包含多个任务信息的JSON数组，批量创建任务。
	/*- 接收一个包含多个任务信息的JSON数组，批量创建任务。  与创建任务不同，这里是直接接受多个任务信息，一整块，然后不断地创建任务。
	//方式：1.创建数组，2.向数组传递信息。3.数组JSON解析 4.接受解析过后的数组。5.for循环遍历数组，不断传入。用并发创建。
	- 实现并发处理，提高导入性能，确保线程安全。*/
}

//------------------------------------------------------------------------

func RegisterHandler(c *gin.Context) {

	db := c.MustGet("db").(*gorm.DB)
	var user models.User
	//必须要想办法  通过某种方式读入用户名 用户密码 （考虑curl 或者 说是url 读取） -》数据绑定
	if err := c.ShouldBindJSON(&user); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		c.Abort()
		return
	} //代表user非空

	//if _,exists
	// 检查用户名是否已存在
	var existingUser models.User
	result := db.First(&existingUser, "username = ?", user.Username)
	if result.RowsAffected == 1 { //存在用户
		c.JSON(http.StatusConflict, gin.H{"error": "Username already exists"})
		c.Abort()
		return
	}
	//用户输入账号密码,注册一个用户的表单，
	//这里要加一个数据库，判断用户是否冲突。冲突不能注册
	//数据库内部可能要加一个表单。
	//好，如果没有问题，就把数据 存入数据库中
	result = db.Create(&user)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": result.Error.Error()})
		return
	}
	//密码是否进行 哈希处理？？？ 防止数据库泄漏
	fmt.Println(user.ID)
	//最后返回一条信息，表示注册成功
	c.JSON(http.StatusCreated, gin.H{"message": "User created"})

}

func LoginHandler(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	var user models.User
	if err := c.ShouldBindJSON(&user); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	}

	//对账号密码进行比对，同样要加入数据库
	//如果不正确: || 以及用户名不存在
	authenticated, err := models.CheckUserCredentials(user.Username, user.Password)
	if err != nil {
		log.Fatalf("error authenticating user: %v", err)
		c.Abort()
		return
	}
	if err = db.Where("username = ?", user.Username).First(&user).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid username or password"})
		return
	} //寻找对应的user
	if authenticated {
		fmt.Println("Authentication successful!")
	} else {
		fmt.Println("Authentication failed.")
		c.Abort()
		return
	}
	//生成JWT令牌
	utils.GenerateToken(user.Username)

	//登录成功
	viper.Set("OwnerID", user.ID)
	c.JSON(http.StatusOK, gin.H{"message": "User login", "UserID": user.ID})
}
