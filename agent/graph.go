package agent

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/compose"
)

func Graph_agent() {
	ctx := context.Background()
	//未添加模型的graph流程 注册图 同时定义输入输出的类型 这里都为string类型
	g := compose.NewGraph[string, string]()
	lambda0 := compose.InvokableLambda(func(ctx context.Context, input string) (output string, err error) {
		if input == "测试1" {
			return "输入1", nil
		} else if input == "测试2" {
			return "输入2", nil
		} else if input == "测试3" {
			return "输入3", nil
		}
		return "", nil
	})
	lambda1 := compose.InvokableLambda(func(ctx context.Context, input string) (output string, err error) {
		return "这里是节点1的输出", nil
	})
	lambda2 := compose.InvokableLambda(func(ctx context.Context, input string) (output string, err error) {
		return "这里是节点2的输出", nil
	})
	lambda3 := compose.InvokableLambda(func(ctx context.Context, input string) (output string, err error) {
		return "这里是节点3的输出", nil
	})
	// 加入节点
	err := g.AddLambdaNode("lambda0", lambda0)
	if err != nil {
		panic(err)
	}
	err = g.AddLambdaNode("lambda1", lambda1)
	if err != nil {
		panic(err)
	}
	err = g.AddLambdaNode("lambda2", lambda2)
	if err != nil {
		panic(err)
	}
	err = g.AddLambdaNode("lambda3", lambda3)
	if err != nil {
		panic(err)
	}
	// 分支连接 增加边 用来标记节点与节点怎样连接的
	err = g.AddEdge(compose.START, "lambda0")
	if err != nil {
		panic(err)
	}
	// 加入分支
	err = g.AddBranch("lambda0", compose.NewGraphBranch(func(ctx context.Context, in string) (endNode string, err error) {
		if in == "输入1" {
			return "lambda1", nil
		} else if in == "输入2" {
			return "lambda2", nil
		} else if in == "输入3" {
			return "lambda3", nil
		}
		// 否则，返回 compose.END，表示流程结束
		return compose.END, nil
	}, map[string]bool{"lambda1": true, "lambda2": true, "lambda3": true, compose.END: true}))
	if err != nil {
		panic(err)
	}
	//增加边 lambda1这个节点直接连接end节点
	err = g.AddEdge("lambda1", compose.END)
	if err != nil {
		panic(err)
	}
	err = g.AddEdge("lambda2", compose.END)
	if err != nil {
		panic(err)
	}
	err = g.AddEdge("lambda3", compose.END)
	if err != nil {
		panic(err)
	}
	// 编译
	r, err := g.Compile(ctx)
	if err != nil {
		panic(err)
	}
	// 执行
	answer, err := r.Invoke(ctx, "测试1")
	if err != nil {
		panic(err)
	}
	fmt.Println(answer)
}
