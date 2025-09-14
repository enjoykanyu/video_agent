package agent

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino-ext/components/model/ollama"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

func NewGraphWithModel() {
	ctx := context.Background()
	//新建图
	g := compose.NewGraph[map[string]string, *schema.Message]()
	lambda := compose.InvokableLambda(func(ctx context.Context, input map[string]string) (output map[string]string, err error) {
		if input["role"] == "gongke" {
			return map[string]string{"role": "gongke", "content": input["content"]}, nil
		}
		if input["role"] == "wenke" {
			return map[string]string{"role": "wenke", "content": input["content"]}, nil
		}
		return map[string]string{"role": "user", "content": input["content"]}, nil
	})
	GongkeLambda := compose.InvokableLambda(func(ctx context.Context, input map[string]string) (output []*schema.Message, err error) {
		return []*schema.Message{
			{
				Role:    schema.System,
				Content: "你是一个专业的工科专业人士，回答问题很严肃认真，不会说废话",
			},
			{
				Role:    schema.User,
				Content: input["content"],
			},
		}, nil
	})
	WenkeLambda := compose.InvokableLambda(func(ctx context.Context, input map[string]string) (output []*schema.Message, err error) {
		return []*schema.Message{
			{
				Role:    schema.System,
				Content: "你是一位专业的文科人士，回答问题很温柔，拥有大量的文科知识",
			},
			{
				Role:    schema.User,
				Content: input["content"],
			},
		}, nil
	})

	model, err := ollama.NewChatModel(ctx, &ollama.ChatModelConfig{
		BaseURL: "http://localhost:11434", // Ollama 服务地址
		Model:   "qwen3:0.6b",             // 模型名称
	})
	if err != nil {
		panic(err)
	}
	//注册节点
	err = g.AddLambdaNode("lambda", lambda)
	if err != nil {
		panic(err)
	}
	err = g.AddLambdaNode("gongke", GongkeLambda)
	if err != nil {
		panic(err)
	}
	err = g.AddLambdaNode("wenke", WenkeLambda)
	if err != nil {
		panic(err)
	}
	err = g.AddChatModelNode("model", model)
	if err != nil {
		panic(err)
	}

	//链接节点 start->lambda
	err = g.AddEdge(compose.START, "lambda")
	if err != nil {
		panic(err)
	}
	//加入分支分之直接把两个lambda节点和branch链接了 lambda-> gongke   lambda->wenke
	g.AddBranch("lambda", compose.NewGraphBranch(func(ctx context.Context, in map[string]string) (endNode string, err error) {
		if in["role"] == "gongke" {
			return "gongke", nil
		}
		if in["role"] == "wenke" {
			return "wenke", nil
		}
		return "wenke", nil
	}, map[string]bool{"wenke": true, "gongke": true}))
	//把两个lambda节点和model节点进行连接 gongke->model
	err = g.AddEdge("gongke", "model")
	if err != nil {
		panic(err)
	}
	// wenke->model
	err = g.AddEdge("wenke", "model")
	if err != nil {
		panic(err)
	}
	//结束节点 model->END
	err = g.AddEdge("model", compose.END)
	if err != nil {
		panic(err)
	}
	//编译
	r, err := g.Compile(ctx)
	if err != nil {
		panic(err)
	}
	input := map[string]string{
		"role":    "wenke",
		"content": "介绍下你自己",
	}
	//执行
	answer, err := r.Invoke(ctx, input)
	if err != nil {
		panic(err)
	}
	fmt.Println(answer.Content)
}
