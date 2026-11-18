# ModelTrace 本地指纹评分

指纹库和评分算法来自 xqy2006/ModelTrace 的提交 `6cd4a8ff0f3f4e49d6d6969dd2fbadfa1569fde2`，作者按 MIT 许可证发布。[固定版本](https://github.com/xqy2006/ModelTrace/tree/6cd4a8ff0f3f4e49d6d6969dd2fbadfa1569fde2) · [许可证](LICENSE)

`data/unified_bank.json` 保留上游原文件，SHA-256 为 `f267d0513aa06981d024ec5394746fceddf44e4e8a75fa6bf8304255f4b32d6d`。Go 代码移植自该提交的 `fingerprint.py`，按有效样本数读取校准参数，在本地计算 18 个候选模型及模型家族的相对概率。模型库随程序编译，作答仅发送到管理员选择的提供商。

`testdata/astra-reference.json` 取自同一版本 `data/gpt_reference.jsonl` 中 `gpt-6-astra` 的前三条公开 `clean` 回答。测试用这些训练数据检查计算结果能否复现。实际账号的归因仍受候选范围、上游采样和系统提示影响。
