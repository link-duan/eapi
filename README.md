# ego-gen-api [WIP]

**本项目正在开发中，暂不能在实际场景中使用**

`ego-gen-api` 通过分析 AST 解析出代码中的路由（方法/路径）和 Handler 函数获取接口的信息（请求响应参数/响应状态码等），进而生成 Swagger 文档。

由于本工具是通过分析 AST 中的必要部分而不执行代码（类似于静态分析）的方式实现的，因此不能覆盖到任一种使用场景。如果你要在项目中使用本工具，您的代码必须必须要按照一定的约定编写，否则将不能生成完整的文档。虽然这给你的代码编写带来了一定的约束，但从另一个角度看，这也使得代码风格更加统一。

本工具暂时只支持了有限的框架（现阶段只支持了 `egin` 的路由）。如果你需要将本项目进行扩展或应用于未支持的框架，本工具实现了插件机制，你可以编写自定义插件。

## 插件 Plugin

> TODO

## 生命周期 Lifecycle
1. Parsing Routes
    1. Method
    2. Path
    3. Handler Name
2. Parsing Handlers
    1. Request Parameters (Query-Parameters/Path-Parameters/Body-Payload)
    2. Response (Status-Code/Data-Type)
