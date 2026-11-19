<template>
  <div class="grid gap-3 sm:grid-cols-3">
    <label class="block">
      <span class="input-label">{{ t('admin.redeem.coupon.currency') }}</span>
      <input v-model="input.currency" type="text" class="input uppercase" maxlength="3" spellcheck="false" required />
    </label>
    <label class="block">
      <span class="input-label">{{ t('admin.redeem.coupon.minimum') }}</span>
      <input v-model="input.minimum" data-testid="recharge-coupon-minimum" type="text" inputmode="decimal" class="input tabular-nums" maxlength="20" required />
    </label>
    <label class="block">
      <span class="input-label">{{ t('admin.redeem.coupon.discount') }}</span>
      <input v-model="input.discount" data-testid="recharge-coupon-discount" type="text" inputmode="decimal" class="input tabular-nums" maxlength="20" required />
    </label>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { RechargeCouponInput } from './rechargeCoupon'

const input = defineModel<RechargeCouponInput>({ required: true })
const { t } = useI18n()
</script>
