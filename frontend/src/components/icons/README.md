# 通用动态图标

页面统一使用 `Icon.vue`，语义名称由 `registry.ts` 导出为 `IconName`。图形文件只包含 SVG 和动效定义，鼠标、焦点、禁用状态及清理由 `useIconAnimation` 管理。各业务页面的品牌标志、用户提供的 SVG 和数据可视化保留自己的入口。

## 来源与许可

- 动态图形来自 [Lucide Animated](https://github.com/pqoqubbw/icons/tree/072c38b1b04ea738d90a084485ccaad4b890ddca/icons)，版本 `072c38b1b04ea738d90a084485ccaad4b890ddca`，MIT 许可见 `LICENSE.lucide-animated`。
- 补充图形来自 [Lucide](https://github.com/lucide-icons/lucide/tree/66d8f9fc394b8530377e5f6112f0b8908ba01280/icons)，版本 `66d8f9fc394b8530377e5f6112f0b8908ba01280`，许可见 `LICENSE.lucide`。每个文件标注其来源。
- 仅保留实际采用的图形源码，运行时无需请求图标 CDN。Vue 动画由 `motion-v` 执行。

## 移植约定

保留上游的路径、关键帧和逐元素过渡。React 的外壳改为统一 Vue SVG 入口；原根 SVG 上的动画移至内部 `g`，外层的尺寸、CSS 旋转及加载状态继续独立生效。上游的循环动画收敛为单次播放，动画序列按完成事件推进。内部路径继承调用方描边，Brain 的描边动画按 `strokeWidth` 比例缩放。

Sparkles 的两组控制器合并为同一播放入口，保留星点的一秒延迟；多关键帧闪烁采用 tween，以适配 Motion 对弹簧关键帧数量的限制。没有官方动画的 Lucide 图形使用 `createFallbackIcon`，在 400ms 内按 `1 → 1.06 → 1` 缩放一次。

新增图标时先选择同语义的上游图形，再在注册表添加名称，不能用无关图形代替缺失的含义。继续维护来源链接和许可文件。

完整交互规范见 [前端 UI 规范](../../../../docs/architecture/frontend_ui_conventions.md)。
