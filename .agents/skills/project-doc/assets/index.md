# Project Doc 目录模板

<!-- 使用时保留一个适用的变体，替换全部占位符，并按仓库主语言改写。 -->

## 根目录变体

~~~markdown
# {{project_name}} 工程文档

> 本文件是 Project Doc 的根目录。当前会话加载 project-doc 技能后，第一次处理本仓库任务时完整读取本文件，再按“读取时机”进入相关分类。之后按技能的“读取与上下文复用”规则复用已读内容，内容缺失或需要刷新时补读。

## 文档范围

{{用一至两句话说明文档库覆盖的长期工程知识，以及哪些内容放在文档库之外。}}

## 根级文档

- [{{document_title}}]({{document_path}})：{{one_sentence_summary}}。读取时机：{{read_when}}。

## 分类

- [架构](architecture/index.md)：{{architecture_summary}}。读取时机：{{architecture_read_when}}。
- [领域](domains/index.md)：{{domains_summary}}。读取时机：{{domains_read_when}}。
- [接口](interfaces/index.md)：{{interfaces_summary}}。读取时机：{{interfaces_read_when}}。
- [运维](operations/index.md)：{{operations_summary}}。读取时机：{{operations_read_when}}。
- [决策](decisions/index.md)：{{decisions_summary}}。读取时机：{{decisions_read_when}}。

<!-- 建库期间保留以下内容。目标文件还没创建时写成代码路径，链接等文件创建后再加。 -->
## 建库状态

- [planned] {{planned_document_path}}：{{planned_scope}}。
- [in_progress] {{current_document_path}}：{{current_scope}}。
~~~

删除没有实际文档的分类，每个保留的分类目录至少有一篇文档。建库完成后，删除“建库状态”章节和所有状态前缀。

## 分类目录变体

~~~markdown
# {{category_name}} 文档目录

> 上级目录：[工程文档](../index.md)

## 范围

{{说明本分类负责哪些知识，以及哪些内容应该路由到其他分类。}}

## 文档

- [{{document_title}}]({{document_file}})：{{one_sentence_summary}}。读取时机：{{read_when}}。

<!-- 建库期间可以使用 planned、in_progress、verified、blocked，建库完成后移除状态。 -->
~~~

每篇文档在一个分类目录里作为规范条目出现。相关分类或相关文档之间用普通链接互相引用，概要写在规范条目里。
