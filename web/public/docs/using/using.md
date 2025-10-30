# 使用说明

## 使用建议

**原则上建议直接使用官方SDK改环境变量访问或者直接使用官方的REST风格的请求BODY构造HTTP请求访问。另外如碰到一些特殊的参数支持情况，可能会需要按照本站风格调用，请参阅下方特殊情况部分**

使用OpenAI SDK的协议调用本站的非OpenAI模型，可能出现参数不支持的情况，或者格式不支持的情况，内部会进行适配。使用时需要自行打一定流量来检测使用效果是否符合预期。
因为从OpenAI格式到其他格式诸如Gemini会有参数转换，如果需要使用OpenAI的格式调多种模型，可参考开源文档https://docs.newapi.pro/api/

## 服务架构和调度策略

为了提供较大的RPM（每分钟请求数），服务端持有一个大的号池，采用多厂商渠道调度策略：

### GPT模型调度优先级
- **Azure** → **OpenAI官方**
- 调用时会优先使用Azure渠道，Azure不可用时自动切换到OpenAI官方

### 各厂商调度优先级
- **Gemini**: Vertex → GCP 
- **GPT**: Azure → OpenAI
- **Claude**: AWS → Anthropic  
- **Doubao**: 火山方舟

  
### 注意事项

#### 1. 第三方渠道模型参数限制
各非直接官方渠道（Azure、Vertex、AWS等）的部分模型参数可能不支持官方的部分参数【claude、openai、google gemini】，遇到此类情况：
- 建议使用前进行小量测试和效果验证，确保参数支持
- 如有确实效果问题，必须优先调度官方，可联系支持

#### 2. 模型版本差异
**所有多供应商渠道都存在此问题**：第三方渠道和官方的不带日期版本模型名对应的具体版本可能不一致：

**GPT示例**：
- Azure上：`gpt-4o-audio-preview` → `gpt-4o-audio-preview-2024-12-17`
- 官方上：`gpt-4o-audio-preview` → `gpt-4o-audio-preview-2025-06-03`


**建议**：如需调用特定版本，请使用带明确版本号的模型名。

## 超时时间设置
统一使用30分钟作为超时时间设置。

## 可用性和重试
### 服务端重试和限流建议
服务端本身会hold客户端打过来的请求链接，默认情况下无限流，如果碰到我们后端容量不充足的模型，例如gemini-2.5-pro之类的，会对对应客户的模型进行限流，此时会根据后端容量的情况动态调整限流阈值，此时客户端表现为请求整体返回耗时上涨，客户端实际成功RPM维持在稳定值。
如有扩容需求，请联系支持。
### 客户端重试建议
由于各家官方可能会不定期封禁号源，因此可用性在官方封禁期间会有一些影响，可能是抖动或者彻底不可用。
建议的重试时间间隔和次数可以相对设多一些，比如10分钟重试一次，总计重试10次，这样整体的兜底时间差不多2小时，一般常规封号情况都会在这个时间内解决。
非常规情况请联系支持。

## 各个厂商官方SDK使用办法

### OpenAI

```python
from openai import OpenAI
import base64

# 初始化 client，传入 api_key 和自定义 base_url
client = OpenAI(
    api_key="sk-xxxx",
    base_url="https://www.furion-tech.com/v1/"
)
```

### Google Gemini

```python
import os
import google.generativeai as genai

# 设置环境变量
os.environ['GOOGLE_API_KEY'] = "sk-xxxx"
os.environ['GOOGLE_GEMINI_BASE_URL'] = "https://www.furion-tech.com/"

# 初始化 Gemini 客户端
client = genai.Client()
```

## 特殊情况  用Gemini 2.5 Pro YouTube URL 视频链接做视频理解
### 特定调用令牌
在API令牌管理页创建令牌，单独用于只调用能传YouTube视频URL的gemini-2.5-pro, 在模型名映射（JSON格式）配置下面的模型名映射，以便流量打到支持视频URL的gcp的号上

![API令牌创建示例](/docs/using/api_create.png)

```json
{"gemini-2.5-pro":"gemini-2.5-pro-youtube"}
```

### gemini-2.5-pro参数支持
`gemini-2.5-pro` 模型在处理 YouTube 视频时需要支持的额外参数，如有不支持的参数请联系支持，请使用以下请求格式：

```python
from google import genai
from google.genai import types
import json,os

os.environ['GOOGLE_API_KEY'] =  "sk-XXXXXXXXXX"
os.environ['GOOGLE_GEMINI_BASE_URL'] = "https://www.furion-tech.com/"

client = genai.Client( )
response = client.models.generate_content(
    model='models/gemini-2.5-pro',
    contents=types.Content(
        parts=[
            types.Part(
                file_data=types.FileData(file_uri='https://www.youtube.com/watch?v=Oy-GxFRzCDo'),
                video_metadata=types.VideoMetadata(fps=1,start_offset='0s',end_offset='3s')
            ),
            types.Part(text=
            """这个视频有多长，讲了什么内容"""
            )
        ]
    ),
    config=types.GenerateContentConfig(
         media_resolution=types.MediaResolution.MEDIA_RESOLUTION_HIGH  
    )
)
print(json.dumps(response, default=str, ensure_ascii=False, indent=2))
```
### gemini 最新版本的思考总结参数兼容
官方最新的python版本生成的http body的思考字段使用的是include_thoughts，go 语言的最新版本是includeThoughts，由于本站是GO写的，
因此会有参数未对齐导致的思考参数未生效，以下提供直接通过extra_body的方式来兼容调用
```python
from google import genai
from google.genai import types
import os,json
os.environ['GOOGLE_API_KEY'] = "sk-xxxxxxxxxxxx"
os.environ['GOOGLE_GEMINI_BASE_URL'] = "https://www.furion-tech.com/"
client = genai.Client()
prompt = "What is the sum of the first 50 prime numbers?"
response = client.models.generate_content(
  model="gemini-2.5-pro",
  contents=prompt,
  config=types.GenerateContentConfig(
    http_options=types.HttpOptions(
      extra_body={
        "generationConfig": {
          "thinkingConfig": {
            "includeThoughts": True
          }
        }
      }
    )
  )
)
# 直接转换为字典并打印JSON
response_dict = response.__dict__ if hasattr(response, '__dict__') else str(response)
print(json.dumps(response_dict, default=str, ensure_ascii=False, indent=2))
```
