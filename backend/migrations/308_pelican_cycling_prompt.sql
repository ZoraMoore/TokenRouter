-- 固定鹈鹕题使用 Sub2API 的骑车题面，运行时拼入 HTML 交付要求。
-- 题面更新后重新领取在途任务，使样本记录与实际发送的题目一致。
UPDATE pelican_test_plans
SET question_prompt = '创建一个 HTML，内容是 SVG 绘制一个鹈鹕骑自行车的 2D 动画，你不需要任何测试',
    plan_revision = plan_revision + 1,
    lease_token = NULL,
    lease_until = NULL
WHERE question_kind = 'pelican'
  AND question_prompt = '创建一个独立 HTML，在一个内联 SVG 中绘制循环的 2D 动画。鹈鹕骑自行车：画出长喙和喉囊、双轮车架、脚踏与腿部联动，背景路标向后移动。直接返回完整 HTML。使用 CSS、SMIL 或内联脚本实现动画，页面所需资源都放在文档中。';
