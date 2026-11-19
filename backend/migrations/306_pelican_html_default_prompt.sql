-- 固定的内置鹈鹕题使用完整 HTML 画布。管理员修改过的题面按计划保存。
UPDATE pelican_test_plans
SET question_prompt = '创建一个独立 HTML，在一个内联 SVG 中绘制循环的 2D 动画。鹈鹕骑自行车：画出长喙和喉囊、双轮车架、脚踏与腿部联动，背景路标向后移动。直接返回完整 HTML。使用 CSS、SMIL 或内联脚本实现动画，页面所需资源都放在文档中。',
    plan_revision = plan_revision + 1,
    lease_token = NULL,
    lease_until = NULL
WHERE question_kind = 'pelican'
  AND question_prompt = '创建一个独立 HTML，在一个内联 SVG 中绘制循环的 2D 动画。鹈鹕骑自行车：画出长喙和喉囊、双轮车架、脚踏与腿部联动，背景路标向后移动。将动画写在 SVG 内，使用 CSS 或 SMIL，图像应能单独预览。直接返回完整代码，所有样式和图形都放在 SVG 内。';
