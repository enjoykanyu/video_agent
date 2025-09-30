package agent

// import (
// 	"context"
// 	"fmt"

// 	"github.com/cloudwego/eino-ext/components/model/ollama"
// 	"github.com/cloudwego/eino/callbacks"
// 	"github.com/cloudwego/eino/compose"
// 	"github.com/cloudwego/eino/schema"
// )

// type State struct {
// 	History map[string]any
// }

// func genFunc(ctx context.Context) *State {
// 	return &State{
// 		History: make(map[string]any),
// 	}
// }

// func OrcGraphWithState(ctx context.Context, input map[string]string) {
// 	g := compose.NewGraph[map[string]string, *schema.Message](
// 		compose.WithGenLocalState(genFunc),
// 	)
// 	lambda := compose.InvokableLambda(func(ctx context.Context, input map[string]string) (output map[string]string, err error) {
// 		//在节点内部处理state // 在这里可以安全地访问和修改state 这个state会在整个图执行过程中保持和传递
// 		//初始化状态数据
// 		_ = compose.ProcessState[*State](ctx, func(_ context.Context, state *State) error {
// 			state.History["test1_action"] = "你叫做张三"
// 			state.History["test2_action"] = "你叫做赵六"
// 			//作为状态数据的初始化节点，为后续节点提供基础数据
// 			return nil
// 		})
// 		if input["role"] == "test1_role" {
// 			return map[string]string{"role": "test1_role", "content": input["content"]}, nil
// 		}
// 		if input["role"] == "test2_role" {
// 			return map[string]string{"role": "test2_role", "content": input["content"]}, nil
// 		}
// 		return map[string]string{"role": "user", "content": input["content"]}, nil
// 	})
// 	//作用：读取并使用状态数据
// 	Test1Lambda := compose.InvokableLambda(func(ctx context.Context, input map[string]string) (output []*schema.Message, err error) {
// 		_ = compose.ProcessState[*State](ctx, func(_ context.Context, state *State) error {
// 			//从state中获取之前设置的"test1_action"
// 			//将动作文本追加到输入内容中
// 			input["content"] = input["content"] + state.History["test1_action"].(string)
// 			return nil
// 		})
// 		return []*schema.Message{
// 			{
// 				Role:    schema.System,
// 				Content: "你是一个优秀的工科人士，每次都会用严谨的语气回答，而且次次回答都会拐到工科知识领域中",
// 			},
// 			{
// 				Role:    schema.User,
// 				Content: input["content"],
// 			},
// 		}, nil
// 	})

// 	Test2Lambda := compose.InvokableLambda(func(ctx context.Context, input map[string]string) (output []*schema.Message, err error) {
// 		// _ = compose.ProcessState[*State](ctx, func(_ context.Context, state *State) error {
// 		// 	input["content"] = input["content"] + state.History["action"].(string)
// 		// 	return nil
// 		// })
// 		return []*schema.Message{
// 			{
// 				Role:    schema.System,
// 				Content: "你是一个优秀的文科人士，每次回答都会用文科知识解释你的问题，不论这个问题是否与文科有关",
// 			},
// 			{
// 				Role:    schema.User,
// 				Content: input["content"],
// 			},
// 		}, nil
// 	})
// 	//使用state节点存储记忆的另一种方式 预处理函数
// 	test2PreHandler := func(ctx context.Context, input map[string]string, state *State) (map[string]string, error) {
// 		input["content"] = input["content"] + state.History["test2_action"].(string)
// 		return input, nil
// 	}

// 	model, err := ollama.NewChatModel(ctx, &ollama.ChatModelConfig{
// 		BaseURL: "http://localhost:11434", // Ollama 服务地址
// 		Model:   "qwen3:0.6b",             // 模型名称
// 	})
// 	if err != nil {
// 		panic(err)
// 	}
// 	//注册节点
// 	err = g.AddLambdaNode("lambda", lambda)
// 	if err != nil {
// 		panic(err)
// 	}
// 	err = g.AddLambdaNode("test1", Test1Lambda)
// 	if err != nil {
// 		panic(err)
// 	}
// 	//WithStatePreHandler 预处理函数
// 	err = g.AddLambdaNode("test2", Test2Lambda, compose.WithStatePreHandler(test2PreHandler))
// 	if err != nil {
// 		panic(err)
// 	}
// 	err = g.AddChatModelNode("model", model)
// 	if err != nil {
// 		panic(err)
// 	}
// 	//加入分支
// 	g.AddBranch("lambda", compose.NewGraphBranch(func(ctx context.Context, in map[string]string) (endNode string, err error) {
// 		if in["role"] == "test1_role" {

// 			return "test1", nil
// 		}
// 		if in["role"] == "test2_role" {
// 			return "test2", nil
// 		}
// 		return "test1", nil
// 	}, map[string]bool{"test1": true, "test2": true}))

// 	//链接节点
// 	err = g.AddEdge(compose.START, "lambda")
// 	if err != nil {
// 		panic(err)
// 	}
// 	err = g.AddEdge("test1", "model")
// 	if err != nil {
// 		panic(err)
// 	}
// 	err = g.AddEdge("test2", "model")
// 	if err != nil {
// 		panic(err)
// 	}
// 	err = g.AddEdge("model", compose.END)
// 	if err != nil {
// 		panic(err)
// 	}
// 	//编译
// 	r, err := g.Compile(ctx)
// 	if err != nil {
// 		panic(err)
// 	}
// 	//执行
// 	answer, err := r.Invoke(ctx, input, compose.WithCallbacks(genCallback()))
// 	if err != nil {
// 		panic(err)
// 	}
// 	fmt.Println(answer.Content)
// }

// func genCallback() callbacks.Handler {
// 	handler := callbacks.NewHandlerBuilder().OnStartFn(func(ctx context.Context, info *callbacks.RunInfo, input callbacks.CallbackInput) context.Context {
// 		fmt.Printf("当前%s节点输入:%s\n", info.Component, input)
// 		return ctx
// 	}).OnEndFn(func(ctx context.Context, info *callbacks.RunInfo, output callbacks.CallbackOutput) context.Context {
// 		fmt.Printf("当前%s节点输出:%s\n", info.Component, output)
// 		return ctx
// 	}).Build()
// 	return handler
// }
