<template>
  <BaseDialog
    :show="show"
    :title="t('admin.tlsFingerprintProfiles.title')"
    width="wide"
    @close="$emit('close')"
  >
    <div class="space-y-4">
      <!-- 头部 -->
      <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <p class="text-sm text-gray-500 dark:text-gray-400">
          {{ t('admin.tlsFingerprintProfiles.description') }}
        </p>
        <button @click="openCreateModal" class="btn btn-primary btn-sm">
          <Icon name="plus" size="sm" class="mr-1" />
          {{ t('admin.tlsFingerprintProfiles.createProfile') }}
        </button>
      </div>

      <!-- 收集器 -->
      <div class="rounded-surface border border-gray-200 bg-gray-50 p-4 dark:border-dark-600 dark:bg-dark-800/60">
        <div class="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
          <div>
            <div class="flex items-center gap-2">
              <Icon name="beaker" size="sm" class="text-primary-600 dark:text-primary-400" />
              <h4 class="text-sm font-medium text-gray-900 dark:text-white">
                {{ t('admin.tlsFingerprintProfiles.collector.title') }}
              </h4>
              <span
                :class="[
                  'rounded-full px-2 py-0.5 text-xs font-medium',
                  isCollectorRunning
                    ? 'bg-green-100 text-green-700 dark:bg-green-900/40 dark:text-green-300'
                    : 'bg-gray-200 text-gray-600 dark:bg-dark-600 dark:text-gray-300'
                ]"
              >
                {{ isCollectorRunning ? t('admin.tlsFingerprintProfiles.collector.running') : t('admin.tlsFingerprintProfiles.collector.stopped') }}
              </span>
            </div>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.tlsFingerprintProfiles.collector.description') }}
            </p>
          </div>
          <div class="flex flex-wrap items-center gap-2">
            <button
              type="button"
              class="btn btn-secondary btn-sm"
              :disabled="collectorLoading"
              @click="loadCollectorStatus"
            >
              <Icon name="refresh" size="sm" :class="['mr-1', collectorLoading ? 'animate-spin' : '']" />
              {{ t('common.refresh') }}
            </button>
            <button
              type="button"
              :class="['btn btn-sm', isCollectorRunning ? 'btn-secondary' : 'btn-primary']"
              :disabled="collectorActionLoading"
              @click="toggleCollector"
            >
              <Icon
                :name="collectorActionLoading ? 'refresh' : (isCollectorRunning ? 'x' : 'play')"
                size="sm"
                :class="['mr-1', collectorActionLoading ? 'animate-spin' : '']"
              />
              {{ isCollectorRunning ? t('admin.tlsFingerprintProfiles.collector.stop') : t('admin.tlsFingerprintProfiles.collector.start') }}
            </button>
          </div>
        </div>

        <div class="mt-3 grid gap-3 text-xs sm:grid-cols-3">
          <div>
            <div class="text-gray-500 dark:text-gray-400">{{ t('admin.tlsFingerprintProfiles.collector.listenAddress') }}</div>
            <div class="mt-1 font-mono text-gray-900 dark:text-gray-100">{{ collectorStatus?.listen_address || '—' }}</div>
          </div>
          <div>
            <div class="text-gray-500 dark:text-gray-400">{{ t('admin.tlsFingerprintProfiles.collector.publicBaseURL') }}</div>
            <div class="mt-1 break-all font-mono text-gray-900 dark:text-gray-100">{{ collectorStatus?.public_base_url || '—' }}</div>
          </div>
          <div>
            <div class="text-gray-500 dark:text-gray-400">{{ t('admin.tlsFingerprintProfiles.collector.certificate') }}</div>
            <div class="mt-1 text-gray-900 dark:text-gray-100">
              {{ collectorStatus?.using_generated_cert ? t('admin.tlsFingerprintProfiles.collector.generatedCert') : t('admin.tlsFingerprintProfiles.collector.configuredCert') }}
            </div>
          </div>
        </div>

        <p v-if="collectorStatus?.last_error" class="mt-3 text-xs text-red-600 dark:text-red-400">
          {{ collectorStatus.last_error }}
        </p>

        <!-- 已认证入站采集链路独立于 CONNECT 收集器，客户正常调用 API 即可进入该链路。 -->
        <section
          class="mt-4 border-t border-gray-200 pt-4 dark:border-dark-600"
          aria-labelledby="authenticated-ingress-title"
          data-testid="authenticated-ingress-status"
        >
          <div class="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
            <div class="min-w-0">
              <div class="flex flex-wrap items-center gap-2">
                <h5 id="authenticated-ingress-title" class="text-sm font-medium text-gray-900 dark:text-white">
                  {{ t('admin.tlsFingerprintProfiles.authenticatedIngress.title') }}
                </h5>
                <span
                  v-if="authenticatedIngressStatus"
                  :class="[
                    'rounded-full px-2 py-0.5 text-xs font-medium',
                    authenticatedIngressStatus.enabled
                      ? 'bg-green-100 text-green-700 dark:bg-green-900/40 dark:text-green-300'
                      : 'bg-gray-200 text-gray-600 dark:bg-dark-600 dark:text-gray-300'
                  ]"
                >
                  {{ authenticatedIngressStatus.enabled
                    ? t('admin.tlsFingerprintProfiles.authenticatedIngress.enabled')
                    : t('admin.tlsFingerprintProfiles.authenticatedIngress.disabled') }}
                </span>
              </div>
              <p class="mt-1 max-w-3xl text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.tlsFingerprintProfiles.authenticatedIngress.description') }}
              </p>
            </div>
            <button
              type="button"
              class="btn btn-secondary btn-xs shrink-0"
              :disabled="authenticatedIngressLoading"
              :aria-label="t('admin.tlsFingerprintProfiles.authenticatedIngress.refresh')"
              data-testid="authenticated-ingress-refresh"
              @click="loadAuthenticatedIngressStatus"
            >
              <Icon name="refresh" size="xs" :class="['mr-1', authenticatedIngressLoading ? 'animate-spin' : '']" />
              {{ t('admin.tlsFingerprintProfiles.authenticatedIngress.refresh') }}
            </button>
          </div>

          <div
            v-if="authenticatedIngressLoading && !authenticatedIngressStatus"
            class="mt-3 flex min-h-16 items-center justify-center gap-2 text-xs text-gray-500 dark:text-gray-400"
            role="status"
            aria-live="polite"
          >
            <Icon name="refresh" size="sm" class="animate-spin" />
            {{ t('admin.tlsFingerprintProfiles.authenticatedIngress.loading') }}
          </div>

          <div
            v-else-if="authenticatedIngressError && !authenticatedIngressStatus"
            class="mt-3 rounded-control border border-red-200 bg-red-50 px-3 py-3 text-xs text-red-700 dark:border-red-900/60 dark:bg-red-900/20 dark:text-red-300"
            role="alert"
            data-testid="authenticated-ingress-error"
          >
            <p class="break-words">{{ authenticatedIngressError }}</p>
            <button type="button" class="btn btn-secondary btn-xs mt-2" @click="loadAuthenticatedIngressStatus">
              {{ t('admin.tlsFingerprintProfiles.authenticatedIngress.retry') }}
            </button>
          </div>

          <template v-else-if="authenticatedIngressStatus">
            <dl
              v-if="hasAuthenticatedIngressActivity"
              class="mt-3 grid gap-px overflow-hidden rounded-control bg-gray-200 sm:grid-cols-2 lg:grid-cols-4 dark:bg-dark-600"
              data-testid="authenticated-ingress-metrics"
            >
              <div
                v-for="metric in authenticatedIngressMetrics"
                :key="metric.key"
                class="min-w-0 bg-white px-3 py-2.5 dark:bg-dark-900/60"
              >
                <dt class="break-words text-xs text-gray-500 dark:text-gray-400">{{ metric.label }}</dt>
                <dd class="mt-1 text-sm font-semibold tabular-nums text-gray-900 dark:text-gray-100">
                  {{ formatCount(metric.value) }}
                </dd>
              </div>
            </dl>

            <div
              v-else
              class="mt-3 rounded-control border border-dashed border-gray-300 px-3 py-4 text-center text-xs text-gray-500 dark:border-dark-600 dark:text-gray-400"
              data-testid="authenticated-ingress-empty"
            >
              {{ authenticatedIngressStatus.enabled
                ? t('admin.tlsFingerprintProfiles.authenticatedIngress.noActivity')
                : t('admin.tlsFingerprintProfiles.authenticatedIngress.disabledHint') }}
            </div>

            <div class="mt-3 flex flex-wrap items-center justify-between gap-x-4 gap-y-1 text-xs text-gray-500 dark:text-gray-400">
              <span>
                {{ t('admin.tlsFingerprintProfiles.authenticatedIngress.lastVerified') }}:
                <span class="text-gray-700 dark:text-gray-300">{{ formatDateTime(authenticatedIngressStatus.last_verified_at) }}</span>
              </span>
              <span v-if="authenticatedIngressLoading" role="status" aria-live="polite">
                {{ t('admin.tlsFingerprintProfiles.authenticatedIngress.refreshing') }}
              </span>
            </div>

            <p
              v-if="authenticatedIngressError"
              class="mt-3 break-words text-xs text-amber-700 dark:text-amber-300"
              role="alert"
              data-testid="authenticated-ingress-refresh-error"
            >
              {{ authenticatedIngressError }}
            </p>
            <p
              v-if="authenticatedIngressStatus.last_error"
              class="mt-3 break-words text-xs text-red-600 dark:text-red-400"
              role="alert"
              data-testid="authenticated-ingress-backend-error"
            >
              {{ t('admin.tlsFingerprintProfiles.authenticatedIngress.lastError') }}:
              {{ authenticatedIngressStatus.last_error }}
            </p>
          </template>
        </section>

        <div v-if="isCollectorRunning" class="mt-4 space-y-3">
          <div class="rounded-control border border-gray-200 bg-white p-3 dark:border-dark-600 dark:bg-dark-900/60">
            <div class="grid gap-3 md:grid-cols-2 lg:grid-cols-4">
              <div>
                <label class="input-label" for="tls-capture-client-kind">{{ t('admin.tlsFingerprintProfiles.collector.clientKind') }}</label>
                <Select
                  id="tls-capture-client-kind"
                  v-model="collectorSessionForm.client_kind"
                  :options="collectorClientKindOptions"
                  :searchable="false"
                  :aria-label="t('admin.tlsFingerprintProfiles.collector.clientKind')"
                />
              </div>
              <div>
                <label class="input-label" for="tls-capture-client-version">{{ t('admin.tlsFingerprintProfiles.collector.clientVersion') }}</label>
                <input id="tls-capture-client-version" v-model="collectorSessionForm.client_version" class="input" maxlength="128" autocomplete="off" placeholder="0.144.1" />
              </div>
              <div>
                <label class="input-label" for="tls-capture-os">{{ t('admin.tlsFingerprintProfiles.collector.operatingSystem') }}</label>
                <input id="tls-capture-os" v-model="collectorSessionForm.operating_system" class="input" maxlength="128" autocomplete="off" placeholder="Ubuntu 22.04" />
              </div>
              <div>
                <label class="input-label" for="tls-capture-arch">{{ t('admin.tlsFingerprintProfiles.collector.architecture') }}</label>
                <input id="tls-capture-arch" v-model="collectorSessionForm.architecture" class="input" maxlength="128" autocomplete="off" placeholder="x86_64" />
              </div>
              <div>
                <label class="input-label" for="tls-capture-runtime">{{ t('admin.tlsFingerprintProfiles.collector.runtime') }}</label>
                <input id="tls-capture-runtime" v-model="collectorSessionForm.runtime" class="input" maxlength="128" autocomplete="off" placeholder="rust / node" />
              </div>
              <div>
                <label class="input-label" for="tls-capture-runtime-version">{{ t('admin.tlsFingerprintProfiles.collector.runtimeVersion') }}</label>
                <input id="tls-capture-runtime-version" v-model="collectorSessionForm.runtime_version" class="input" maxlength="128" autocomplete="off" placeholder="24.3.0" />
              </div>
              <div class="md:col-span-2">
                <label class="input-label" for="tls-capture-target">{{ t('admin.tlsFingerprintProfiles.collector.targetHost') }}</label>
                <Select
                  id="tls-capture-target"
                  v-model="collectorSessionForm.target_host"
                  :options="collectorTargetOptions"
                  :searchable="false"
                  :aria-label="t('admin.tlsFingerprintProfiles.collector.targetHost')"
                />
              </div>
            </div>
            <label class="mt-3 flex min-h-11 cursor-pointer items-center gap-3 rounded-control bg-gray-50 px-3 py-2 text-sm text-gray-700 dark:bg-dark-800 dark:text-gray-200">
              <input v-model="collectorSessionForm.cold_start_confirmed" type="checkbox" class="h-4 w-4 rounded-compact border-gray-300 text-primary-600 focus:ring-primary-500" />
              <span>{{ t('admin.tlsFingerprintProfiles.collector.coldStartConfirmed') }}</span>
            </label>
            <p class="mt-2 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.tlsFingerprintProfiles.collector.controlledHint') }}
            </p>
            <p v-if="collectorSessionError" class="mt-2 text-xs text-red-600 dark:text-red-400" role="alert">
              {{ collectorSessionError }}
            </p>
          </div>

          <div class="flex flex-wrap items-center gap-2">
            <button
              type="button"
              class="btn btn-primary btn-sm"
              :disabled="collectorSessionLoading"
              @click="createCollectorSession"
            >
              <Icon name="plus" size="sm" class="mr-1" />
              {{ t('admin.tlsFingerprintProfiles.collector.createSession') }}
            </button>
            <button
              v-if="collectorSession"
              type="button"
              class="btn btn-secondary btn-sm"
              :disabled="collectorCapturesLoading"
              @click="refreshCollectorCaptures"
            >
              <Icon name="refresh" size="sm" :class="['mr-1', collectorCapturesLoading ? 'animate-spin' : '']" />
              {{ t('admin.tlsFingerprintProfiles.collector.refreshCaptures') }}
            </button>
          </div>

          <div v-if="collectorSession" class="space-y-3 rounded-control border border-gray-200 bg-white p-3 dark:border-dark-600 dark:bg-dark-900/60">
            <div class="grid gap-3 text-xs sm:grid-cols-2">
              <div>
                <div class="text-gray-500 dark:text-gray-400">{{ t('admin.tlsFingerprintProfiles.collector.proxyURL') }}</div>
                <div class="mt-1 break-all font-mono text-gray-900 dark:text-gray-100">{{ collectorSession.proxy_url || '—' }}</div>
              </div>
              <div>
                <div class="text-gray-500 dark:text-gray-400">{{ t('admin.tlsFingerprintProfiles.collector.expiresAt') }}</div>
                <div class="mt-1 text-gray-900 dark:text-gray-100">{{ formatDateTime(collectorSession.expires_at) }}</div>
              </div>
              <div>
                <div class="text-gray-500 dark:text-gray-400">{{ t('admin.tlsFingerprintProfiles.collector.targetHost') }}</div>
                <div class="mt-1 font-mono text-gray-900 dark:text-gray-100">{{ collectorSession.target_host || '—' }}</div>
              </div>
              <div>
                <div class="text-gray-500 dark:text-gray-400">{{ t('admin.tlsFingerprintProfiles.collector.declaredClient') }}</div>
                <div class="mt-1 text-gray-900 dark:text-gray-100">
                  {{ formatClientKind(collectorSession.client_kind || '') }} {{ collectorSession.client_version || '' }} · {{ collectorSession.operating_system || '—' }} / {{ collectorSession.architecture || '—' }}
                </div>
              </div>
            </div>

            <div class="min-w-0">
              <div class="mb-1 flex items-center justify-between gap-3">
                <span class="text-xs font-medium text-gray-700 dark:text-gray-300">{{ t('admin.tlsFingerprintProfiles.collector.controlledCommand') }}</span>
                <button type="button" class="btn btn-secondary btn-xs" @click="copyText(controlledCaptureCommand)">
                  <Icon name="copy" size="xs" class="mr-1" />
                  {{ t('common.copy') }}
                </button>
              </div>
              <pre class="max-h-32 overflow-y-auto whitespace-pre-wrap break-all rounded-compact bg-gray-900 p-2 text-xs leading-relaxed text-gray-100">{{ controlledCaptureCommand }}</pre>
              <p class="mt-1 text-xs text-amber-700 dark:text-amber-300">
                {{ t('admin.tlsFingerprintProfiles.collector.commandStopsBeforeHTTP') }}
              </p>
            </div>

            <div v-if="activeCAPEM" class="flex flex-wrap items-center gap-2">
              <button type="button" class="btn btn-secondary btn-sm" @click="copyText(activeCAPEM)">
                <Icon name="copy" size="sm" class="mr-1" />
                {{ t('admin.tlsFingerprintProfiles.collector.copyCA') }}
              </button>
              <button type="button" class="btn btn-secondary btn-sm" @click="downloadCA">
                <Icon name="download" size="sm" class="mr-1" />
                {{ t('admin.tlsFingerprintProfiles.collector.downloadCA') }}
              </button>
            </div>
          </div>

          <div v-if="collectorSession" class="space-y-2" data-testid="tls-collector-captures">
            <div class="flex items-center justify-between">
              <h5 class="text-xs font-medium uppercase text-gray-500 dark:text-gray-400">
                {{ t('admin.tlsFingerprintProfiles.collector.captures') }}
              </h5>
              <span class="text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.tlsFingerprintProfiles.collector.captureCount', { count: collectorCaptures.length }) }}
              </span>
            </div>
            <div v-if="collectorCaptures.length === 0" class="rounded-control border border-dashed border-gray-300 px-3 py-4 text-center text-xs text-gray-500 dark:border-dark-600 dark:text-gray-400">
              {{ t('admin.tlsFingerprintProfiles.collector.noCaptures') }}
            </div>
            <div
              v-for="record in collectorCaptures"
              :key="record.id"
              class="rounded-control border border-gray-200 bg-white p-3 dark:border-dark-600 dark:bg-dark-900/60"
            >
              <div class="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                <div class="min-w-0 text-xs">
                  <div class="flex flex-wrap items-center gap-2">
                    <span class="font-medium text-gray-900 dark:text-white">{{ formatClientKind(record.client_kind) }}</span>
                    <span class="font-mono text-gray-500 dark:text-gray-400">{{ record.ja3_hash }}</span>
                    <span
                      :class="[
                        'rounded-full px-2 py-0.5 text-xs font-medium',
                        record.promotion_eligible
                          ? 'bg-green-100 text-green-700 dark:bg-green-900/40 dark:text-green-300'
                          : 'bg-amber-100 text-amber-800 dark:bg-amber-900/40 dark:text-amber-200'
                      ]"
                    >
                      {{ record.promotion_eligible ? t('admin.tlsFingerprintProfiles.collector.ready') : t('admin.tlsFingerprintProfiles.collector.blocked') }}
                    </span>
                  </div>
                  <div class="mt-1 break-all text-gray-500 dark:text-gray-400">
                    {{ record.user_agent || '—' }}
                  </div>
                  <div class="mt-1 text-gray-500 dark:text-gray-400">
                    {{ formatDateTime(record.captured_at) }} · {{ record.target_host || '—' }} · {{ t('admin.tlsFingerprintProfiles.collector.matchingSamples', { count: record.matching_sample_count || 0 }) }}
                  </div>
                  <ul v-if="record.blocked_reasons?.length" class="mt-2 space-y-1 text-amber-700 dark:text-amber-300" role="status">
                    <li v-for="reason in record.blocked_reasons" :key="reason" class="break-words">{{ reason }}</li>
                  </ul>
                  <ul v-if="record.replay_validation?.warnings?.length" class="mt-2 space-y-1 text-gray-500 dark:text-gray-400">
                    <li v-for="warning in record.replay_validation.warnings" :key="warning.code" class="break-words">{{ warning.message }}</li>
                  </ul>
                </div>
                <div class="flex flex-wrap items-center gap-2">
                  <button type="button" class="btn btn-secondary btn-xs" @click="copyText(record.yaml)">
                    <Icon name="copy" size="xs" class="mr-1" />
                    {{ t('admin.tlsFingerprintProfiles.collector.copyYaml') }}
                  </button>
                  <button
                    type="button"
                    class="btn btn-primary btn-xs"
                    :disabled="!record.promotion_eligible || promotingCaptureId === record.id"
                    @click="promoteCapture(record)"
                  >
                    <Icon :name="promotingCaptureId === record.id ? 'refresh' : 'check'" size="xs" :class="['mr-1', promotingCaptureId === record.id ? 'animate-spin' : '']" />
                    {{ t('admin.tlsFingerprintProfiles.collector.promoteCapture') }}
                  </button>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- 模板列表 -->
      <div v-if="loading" class="flex items-center justify-center py-8">
        <Icon name="refresh" size="lg" class="animate-spin text-gray-400" />
      </div>

      <div v-else-if="profiles.length === 0" class="py-8 text-center">
        <div class="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-gray-100 dark:bg-dark-700">
          <Icon name="shield" size="lg" class="text-gray-400" />
        </div>
        <h4 class="mb-1 text-sm font-medium text-gray-900 dark:text-white">
          {{ t('admin.tlsFingerprintProfiles.noProfiles') }}
        </h4>
        <p class="text-sm text-gray-500 dark:text-gray-400">
          {{ t('admin.tlsFingerprintProfiles.createFirstProfile') }}
        </p>
      </div>

      <div v-else class="max-h-96 overflow-auto rounded-surface border border-gray-200 dark:border-dark-600">
        <table class="min-w-full divide-y divide-gray-200 dark:divide-dark-700">
          <thead class="sticky top-0 bg-gray-50 dark:bg-dark-700">
            <tr>
              <th class="px-3 py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400">
                {{ t('admin.tlsFingerprintProfiles.columns.name') }}
              </th>
              <th class="px-3 py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400">
                {{ t('admin.tlsFingerprintProfiles.columns.description') }}
              </th>
              <th class="px-3 py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400">
                {{ t('admin.tlsFingerprintProfiles.columns.validation') }}
              </th>
              <th class="px-3 py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400">
                {{ t('admin.tlsFingerprintProfiles.columns.grease') }}
              </th>
              <th class="px-3 py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400">
                {{ t('admin.tlsFingerprintProfiles.columns.alpn') }}
              </th>
              <th class="px-3 py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400">
                {{ t('admin.tlsFingerprintProfiles.columns.actions') }}
              </th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-200 bg-white dark:divide-dark-700 dark:bg-dark-800">
            <tr v-for="profile in profiles" :key="profile.id" class="hover:bg-gray-50 dark:hover:bg-dark-700">
              <td class="px-3 py-2">
                <div class="font-medium text-gray-900 dark:text-white text-sm">{{ profile.name }}</div>
              </td>
              <td class="px-3 py-2">
                <div v-if="profile.description" class="text-sm text-gray-500 dark:text-gray-400 max-w-xs truncate">
                  {{ profile.description }}
                </div>
                <div v-else class="text-xs text-gray-400 dark:text-gray-600">—</div>
              </td>
              <td class="px-3 py-2">
                <span :class="['rounded-full px-2 py-0.5 text-xs font-medium', validationStatusClass(profile.verification?.validation_status)]">
                  {{ formatValidationStatus(profile.verification?.validation_status) }}
                </span>
                <div
                  class="mt-1 text-xs text-gray-500 dark:text-gray-400"
                  :data-testid="`tls-profile-source-${profile.id}`"
                >
                  {{ formatVerificationSource(profile.verification?.source_kind) }}
                </div>
                <div
                  v-if="profile.verification?.validation_status === 'verified' && !profile.verification?.approved_for_use"
                  class="mt-1 text-xs font-medium text-amber-700 dark:text-amber-300"
                >
                  {{ t('admin.tlsFingerprintProfiles.approval.pending') }}
                </div>
                <div v-if="profile.verification?.capture_target" class="mt-1 font-mono text-xs text-gray-500 dark:text-gray-400">
                  {{ profile.verification.capture_target }}
                </div>
              </td>
              <td class="px-3 py-2">
                <Icon
                  :name="profile.enable_grease ? 'check' : 'lock'"
                  size="sm"
                  :class="profile.enable_grease ? 'text-green-500' : 'text-gray-400'"
                />
              </td>
              <td class="px-3 py-2">
                <div v-if="profile.alpn_protocols?.length" class="flex flex-wrap gap-1">
                  <span
                    v-for="proto in profile.alpn_protocols.slice(0, 3)"
                    :key="proto"
                    class="badge badge-primary text-xs"
                  >
                    {{ proto }}
                  </span>
                  <span v-if="profile.alpn_protocols.length > 3" class="text-xs text-gray-500">
                    +{{ profile.alpn_protocols.length - 3 }}
                  </span>
                </div>
                <div v-else class="text-xs text-gray-400 dark:text-gray-600">—</div>
              </td>
              <td class="px-3 py-2">
                <div class="flex items-center gap-1">
                  <button
                    v-if="profile.verification?.validation_status === 'verified' && !profile.verification?.approved_for_use"
                    type="button"
                    class="btn btn-secondary btn-xs"
                    :disabled="approvingProfileId === profile.id"
                    @click="approveProfile(profile)"
                  >
                    <Icon name="check" size="xs" :class="['mr-1', approvingProfileId === profile.id ? 'animate-pulse' : '']" />
                    {{ t('admin.tlsFingerprintProfiles.approval.approve') }}
                  </button>
                  <button
                    @click="handleEdit(profile)"
                    class="p-1 text-gray-500 hover:text-primary-600 dark:hover:text-primary-400"
                    :title="t('common.edit')"
                    :aria-label="t('common.edit')"
                  >
                    <Icon name="edit" size="sm" />
                  </button>
                  <button
                    type="button"
                    @click="handleCopyYaml(profile)"
                    class="p-1 text-gray-500 hover:text-primary-600 dark:hover:text-primary-400"
                    :title="t('admin.tlsFingerprintProfiles.copyYaml')"
                    :aria-label="t('admin.tlsFingerprintProfiles.copyYaml')"
                  >
                    <Icon name="copy" size="sm" />
                  </button>
                  <button
                    @click="handleDelete(profile)"
                    class="p-1 text-gray-500 hover:text-red-600 dark:hover:text-red-400"
                    :title="t('common.delete')"
                    :aria-label="t('common.delete')"
                  >
                    <Icon name="trash" size="sm" />
                  </button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <template #footer>
      <div class="flex justify-end">
        <button @click="$emit('close')" class="btn btn-secondary">
          {{ t('common.close') }}
        </button>
      </div>
    </template>

    <!-- 创建/编辑弹窗 -->
    <BaseDialog
      :show="showCreateModal || showEditModal"
      :title="showEditModal ? t('admin.tlsFingerprintProfiles.editProfile') : t('admin.tlsFingerprintProfiles.createProfile')"
      width="wide"
      :z-index="60"
      @close="closeFormModal"
    >
      <form @submit.prevent="handleSubmit" class="space-y-4">
        <div class="rounded-control bg-amber-50 px-3 py-2 text-sm text-amber-800 dark:bg-amber-900/30 dark:text-amber-200" role="status">
          {{ t('admin.tlsFingerprintProfiles.form.manualDraftWarning') }}
        </div>
        <!-- 粘贴 YAML -->
        <div>
          <label class="input-label">{{ t('admin.tlsFingerprintProfiles.form.pasteYaml') }}</label>
          <textarea
            v-model="yamlInput"
            rows="4"
            class="input font-mono text-xs"
            :placeholder="t('admin.tlsFingerprintProfiles.form.pasteYamlPlaceholder')"
            @paste="handleYamlPaste"
          />
          <div class="mt-1 flex items-center gap-2">
            <button type="button" @click="parseYamlInput" class="btn btn-secondary btn-sm">
              {{ t('admin.tlsFingerprintProfiles.form.parseYaml') }}
            </button>
            <p class="text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.tlsFingerprintProfiles.form.pasteYamlHint') }}
              <a href="https://tls.sub2api.org" target="_blank" rel="noopener noreferrer" class="text-primary-600 hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300 underline">{{ t('admin.tlsFingerprintProfiles.form.openCollector') }}</a>
            </p>
          </div>
        </div>

        <hr class="border-gray-200 dark:border-dark-600" />

        <!-- 基础信息 -->
        <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div>
            <label class="input-label">{{ t('admin.tlsFingerprintProfiles.form.name') }}</label>
            <input
              v-model="form.name"
              type="text"
              required
              class="input"
              :placeholder="t('admin.tlsFingerprintProfiles.form.namePlaceholder')"
            />
          </div>
          <div>
            <label class="input-label">{{ t('admin.tlsFingerprintProfiles.form.description') }}</label>
            <input
              v-model="form.description"
              type="text"
              class="input"
              :placeholder="t('admin.tlsFingerprintProfiles.form.descriptionPlaceholder')"
            />
          </div>
        </div>

        <!-- GREASE 开关 -->
        <div class="flex items-center gap-3">
          <button
            type="button"
            @click="form.enable_grease = !form.enable_grease"
            :aria-label="t('admin.tlsFingerprintProfiles.form.enableGrease')"
            :aria-pressed="form.enable_grease"
            :class="[
              'relative inline-flex h-5 w-9 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
              form.enable_grease ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
            ]"
          >
            <span
              :class="[
                'pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
                form.enable_grease ? 'translate-x-4' : 'translate-x-0'
              ]"
            />
          </button>
          <div>
            <span class="text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t('admin.tlsFingerprintProfiles.form.enableGrease') }}
            </span>
            <p class="text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.tlsFingerprintProfiles.form.enableGreaseHint') }}
            </p>
          </div>
        </div>

        <!-- TLS 数组字段 -->
        <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div>
            <label class="input-label text-xs">{{ t('admin.tlsFingerprintProfiles.form.cipherSuites') }}</label>
            <textarea
              v-model="fieldInputs.cipher_suites"
              rows="2"
              class="input font-mono text-xs"
              :placeholder="'0x1301, 0x1302, 0xc02c'"
            />
            <p class="input-hint text-xs">{{ t('admin.tlsFingerprintProfiles.form.cipherSuitesHint') }}</p>
          </div>

          <div>
            <label class="input-label text-xs">{{ t('admin.tlsFingerprintProfiles.form.curves') }}</label>
            <textarea
              v-model="fieldInputs.curves"
              rows="2"
              class="input font-mono text-xs"
              :placeholder="'29, 23, 24'"
            />
            <p class="input-hint text-xs">{{ t('admin.tlsFingerprintProfiles.form.curvesHint') }}</p>
          </div>

          <div>
            <label class="input-label text-xs">{{ t('admin.tlsFingerprintProfiles.form.signatureAlgorithms') }}</label>
            <textarea
              v-model="fieldInputs.signature_algorithms"
              rows="2"
              class="input font-mono text-xs"
              :placeholder="'0x0403, 0x0804, 0x0401'"
            />
          </div>

          <div>
            <label class="input-label text-xs">{{ t('admin.tlsFingerprintProfiles.form.signatureAlgorithmsCert') }}</label>
            <textarea
              v-model="fieldInputs.signature_algorithms_cert"
              rows="2"
              class="input font-mono text-xs"
              :placeholder="'0x0403, 0x0804, 0x0401'"
            />
          </div>

          <div>
            <label class="input-label text-xs">{{ t('admin.tlsFingerprintProfiles.form.supportedVersions') }}</label>
            <textarea
              v-model="fieldInputs.supported_versions"
              rows="2"
              class="input font-mono text-xs"
              :placeholder="'0x0304, 0x0303'"
            />
          </div>

          <div>
            <label class="input-label text-xs">{{ t('admin.tlsFingerprintProfiles.form.keyShareGroups') }}</label>
            <textarea
              v-model="fieldInputs.key_share_groups"
              rows="2"
              class="input font-mono text-xs"
              :placeholder="'29, 23'"
            />
          </div>

          <div>
            <label class="input-label text-xs">{{ t('admin.tlsFingerprintProfiles.form.extensions') }}</label>
            <textarea
              v-model="fieldInputs.extensions"
              rows="2"
              class="input font-mono text-xs"
              :placeholder="'0x0000, 0x0005, 0x000a'"
            />
          </div>

          <div>
            <label class="input-label text-xs">{{ t('admin.tlsFingerprintProfiles.form.pointFormats') }}</label>
            <textarea
              v-model="fieldInputs.point_formats"
              rows="2"
              class="input font-mono text-xs"
              :placeholder="'0'"
            />
          </div>

          <div>
            <label class="input-label text-xs">{{ t('admin.tlsFingerprintProfiles.form.pskModes') }}</label>
            <textarea
              v-model="fieldInputs.psk_modes"
              rows="2"
              class="input font-mono text-xs"
              :placeholder="'1'"
            />
          </div>
        </div>

        <!-- ALPN 协议 -->
        <div>
          <label class="input-label text-xs">{{ t('admin.tlsFingerprintProfiles.form.alpnProtocols') }}</label>
          <textarea
            v-model="fieldInputs.alpn_protocols"
            rows="2"
            class="input font-mono text-xs"
            :placeholder="'h2, http/1.1'"
          />
        </div>
      </form>

      <template #footer>
        <div class="flex justify-end gap-3">
          <button @click="closeFormModal" type="button" class="btn btn-secondary">
            {{ t('common.cancel') }}
          </button>
          <button @click="handleSubmit" :disabled="submitting" class="btn btn-primary">
            <Icon v-if="submitting" name="refresh" size="sm" class="mr-1 animate-spin" />
            {{ showEditModal ? t('common.update') : t('common.create') }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <!-- 删除确认 -->
    <ConfirmDialog
      :show="showDeleteDialog"
      :title="t('admin.tlsFingerprintProfiles.deleteProfile')"
      :message="t('admin.tlsFingerprintProfiles.deleteConfirmMessage', { name: deletingProfile?.name })"
      :confirm-text="t('common.delete')"
      :cancel-text="t('common.cancel')"
      :danger="true"
      @confirm="confirmDelete"
      @cancel="showDeleteDialog = false"
    />
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, onUnmounted, ref, reactive, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api/admin'
import type {
  CreateTLSFingerprintCollectorSessionRequest,
  TLSFingerprintCaptureRecord,
  TLSFingerprintAuthenticatedIngressStatus,
  TLSFingerprintCollectorSession,
  TLSFingerprintCollectorStatus,
  TLSFingerprintProfile
} from '@/api/admin/tlsFingerprintProfile'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Select, { type SelectOption } from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'

const props = defineProps<{
  show: boolean
}>()

const emit = defineEmits<{
  close: []
}>()

void emit // 模板中通过 $emit 使用，脚本侧保留引用避免类型告警。

const { t } = useI18n()
const appStore = useAppStore()

const profiles = ref<TLSFingerprintProfile[]>([])
const loading = ref(false)
const submitting = ref(false)
const showCreateModal = ref(false)
const showEditModal = ref(false)
const showDeleteDialog = ref(false)
const editingProfile = ref<TLSFingerprintProfile | null>(null)
const deletingProfile = ref<TLSFingerprintProfile | null>(null)
const approvingProfileId = ref<number | null>(null)
const yamlInput = ref('')
const collectorStatus = ref<TLSFingerprintCollectorStatus | null>(null)
const authenticatedIngressStatus = ref<TLSFingerprintAuthenticatedIngressStatus | null>(null)
const collectorSession = ref<TLSFingerprintCollectorSession | null>(null)
const collectorCaptures = ref<TLSFingerprintCaptureRecord[]>([])
const collectorLoading = ref(false)
const authenticatedIngressLoading = ref(false)
const authenticatedIngressError = ref('')
const collectorActionLoading = ref(false)
const collectorSessionLoading = ref(false)
const collectorCapturesLoading = ref(false)
const promotingCaptureId = ref<string | null>(null)
const collectorSessionError = ref('')
let collectorPollTimer: number | null = null

const collectorSessionForm = reactive<CreateTLSFingerprintCollectorSessionRequest>({
  mode: 'controlled_connect',
  client_kind: 'codex',
  client_version: '',
  runtime: '',
  runtime_version: '',
  operating_system: '',
  architecture: '',
  target_host: 'chatgpt.com',
  cold_start_confirmed: false
})

const collectorClientKindOptions = computed<SelectOption[]>(() => [
  { value: 'codex', label: 'Codex CLI' },
  { value: 'node', label: 'Node.js' },
  { value: 'claude_code', label: 'Claude Code' }
])
const collectorTargetOptions = computed<SelectOption[]>(() => [
  { value: 'chatgpt.com', label: t('admin.tlsFingerprintProfiles.collector.targetChatGPT') },
  { value: 'api.openai.com', label: t('admin.tlsFingerprintProfiles.collector.targetOpenAIAPI') }
])
const canCreateCollectorSession = computed(() => Boolean(
  collectorSessionForm.client_kind &&
  collectorSessionForm.client_version.trim() &&
  collectorSessionForm.operating_system.trim() &&
  collectorSessionForm.architecture.trim() &&
  collectorSessionForm.target_host &&
  collectorSessionForm.cold_start_confirmed
))
const authenticatedIngressMetrics = computed(() => {
  const status = authenticatedIngressStatus.value
  return [
    { key: 'pending_client_hellos', label: t('admin.tlsFingerprintProfiles.authenticatedIngress.pendingClientHellos'), value: status?.pending_client_hellos },
    { key: 'pending_authenticated_flows', label: t('admin.tlsFingerprintProfiles.authenticatedIngress.pendingAuthenticatedFlows'), value: status?.pending_authenticated_flows },
    { key: 'matched_flows', label: t('admin.tlsFingerprintProfiles.authenticatedIngress.matchedFlows'), value: status?.matched_flows },
    { key: 'rejected_flows', label: t('admin.tlsFingerprintProfiles.authenticatedIngress.rejectedFlows'), value: status?.rejected_flows },
    { key: 'candidate_groups', label: t('admin.tlsFingerprintProfiles.authenticatedIngress.candidateGroups'), value: status?.candidate_groups },
    { key: 'validations_in_flight', label: t('admin.tlsFingerprintProfiles.authenticatedIngress.validationsInFlight'), value: status?.validations_in_flight },
    { key: 'verified_profiles_created', label: t('admin.tlsFingerprintProfiles.authenticatedIngress.verifiedProfilesCreated'), value: status?.verified_profiles_created },
    { key: 'existing_profiles_matched', label: t('admin.tlsFingerprintProfiles.authenticatedIngress.existingProfilesMatched'), value: status?.existing_profiles_matched }
  ]
})
const hasAuthenticatedIngressActivity = computed(() => (
  authenticatedIngressMetrics.value.some(metric => Number(metric.value || 0) > 0) ||
  Boolean(authenticatedIngressStatus.value?.last_verified_at)
))

// 数组字段以字符串形式编辑，提交前再统一解析。
const fieldInputs = reactive({
  cipher_suites: '',
  curves: '',
  point_formats: '',
  signature_algorithms: '',
  signature_algorithms_cert: '',
  alpn_protocols: '',
  supported_versions: '',
  key_share_groups: '',
  psk_modes: '',
  extensions: ''
})

const form = reactive({
  name: '',
  description: null as string | null,
  enable_grease: false
})

const isCollectorRunning = computed(() => Boolean(collectorStatus.value?.running))
const activeCAPEM = computed(() => collectorSession.value?.ca_pem || collectorStatus.value?.ca_pem || '')
const controlledCaptureCommand = computed(() => {
  const session = collectorSession.value
  if (!session?.proxy_url) return ''
  const proxy = JSON.stringify(session.proxy_url)
  if (session.client_kind === 'node') {
    const ca = activeCAPEM.value ? 'NODE_EXTRA_CA_CERTS=/path/to/tokenrouter-tls-collector-ca.pem ' : ''
    return `${ca}HTTPS_PROXY=${proxy} https_proxy=${proxy} node your-script.js`
  }
  if (session.client_kind === 'claude_code') {
    const ca = activeCAPEM.value ? 'NODE_EXTRA_CA_CERTS=/path/to/tokenrouter-tls-collector-ca.pem ' : ''
    return `${ca}HTTPS_PROXY=${proxy} https_proxy=${proxy} claude "test"`
  }
  const ca = activeCAPEM.value ? 'CODEX_CA_CERTIFICATE=/path/to/tokenrouter-tls-collector-ca.pem ' : ''
  return `${ca}HTTPS_PROXY=${proxy} https_proxy=${proxy} codex`
})

// 弹窗打开时刷新模板列表与收集器状态。
watch(() => props.show, (newVal) => {
  if (newVal) {
    loadProfiles()
    loadCollectorStatus()
    loadAuthenticatedIngressStatus()
  } else {
    stopCollectorPolling()
  }
}, { immediate: true })

onUnmounted(() => {
  stopCollectorPolling()
})

async function loadProfiles() {
	loading.value = true
	try {
		profiles.value = await adminAPI.tlsFingerprintProfiles.list()
	} catch (error) {
    appStore.showError(t('admin.tlsFingerprintProfiles.loadFailed'))
    console.error('Error loading TLS fingerprint profiles:', error)
	} finally {
		loading.value = false
	}
}

const resetForm = () => {
  form.name = ''
  form.description = null
  form.enable_grease = false
  fieldInputs.cipher_suites = ''
  fieldInputs.curves = ''
  fieldInputs.point_formats = ''
  fieldInputs.signature_algorithms = ''
  fieldInputs.signature_algorithms_cert = ''
  fieldInputs.alpn_protocols = ''
  fieldInputs.supported_versions = ''
  fieldInputs.key_share_groups = ''
  fieldInputs.psk_modes = ''
  fieldInputs.extensions = ''
  yamlInput.value = ''
}

const openCreateModal = () => {
  resetForm()
  showCreateModal.value = true
}

async function loadCollectorStatus() {
	collectorLoading.value = true
	try {
		collectorStatus.value = await adminAPI.tlsFingerprintProfiles.collectorStatus()
	} catch (error) {
    appStore.showError(t('admin.tlsFingerprintProfiles.collector.statusFailed'))
    console.error('Error loading TLS fingerprint collector status:', error)
	} finally {
		collectorLoading.value = false
	}
}

// 已认证入站状态单独失败时保留旧快照，并在区块内提供重试入口。
async function loadAuthenticatedIngressStatus() {
  authenticatedIngressLoading.value = true
  authenticatedIngressError.value = ''
  try {
    authenticatedIngressStatus.value = await adminAPI.tlsFingerprintProfiles.authenticatedIngressStatus()
  } catch (error: any) {
    authenticatedIngressError.value = error?.response?.data?.detail || error?.message || t('admin.tlsFingerprintProfiles.authenticatedIngress.loadFailed')
    console.error('Error loading authenticated ingress TLS status:', error)
  } finally {
    authenticatedIngressLoading.value = false
  }
}

const toggleCollector = async () => {
  collectorActionLoading.value = true
  try {
    if (isCollectorRunning.value) {
      await adminAPI.tlsFingerprintProfiles.stopCollector()
      collectorSession.value = null
      collectorCaptures.value = []
      stopCollectorPolling()
      appStore.showSuccess(t('admin.tlsFingerprintProfiles.collector.stopSuccess'))
    } else {
      collectorStatus.value = await adminAPI.tlsFingerprintProfiles.startCollector()
      appStore.showSuccess(t('admin.tlsFingerprintProfiles.collector.startSuccess'))
    }
    await loadCollectorStatus()
  } catch (error: any) {
    const message = error?.message || error?.response?.data?.detail || t('admin.tlsFingerprintProfiles.collector.actionFailed')
    appStore.showError(message)
    console.error('Error toggling TLS fingerprint collector:', error)
  } finally {
    collectorActionLoading.value = false
  }
}

const createCollectorSession = async () => {
  collectorSessionError.value = ''
  if (!canCreateCollectorSession.value) {
    collectorSessionError.value = t('admin.tlsFingerprintProfiles.collector.sessionValidationError')
    return
  }
  collectorSessionLoading.value = true
  try {
    collectorSession.value = await adminAPI.tlsFingerprintProfiles.createCollectorSession({
      ...collectorSessionForm,
      client_version: collectorSessionForm.client_version.trim(),
      runtime: collectorSessionForm.runtime?.trim() || undefined,
      runtime_version: collectorSessionForm.runtime_version?.trim() || undefined,
      operating_system: collectorSessionForm.operating_system.trim(),
      architecture: collectorSessionForm.architecture.trim()
    })
    collectorCaptures.value = []
    startCollectorPolling()
    appStore.showSuccess(t('admin.tlsFingerprintProfiles.collector.sessionCreated'))
  } catch (error: any) {
    const message = error?.message || error?.response?.data?.detail || t('admin.tlsFingerprintProfiles.collector.sessionFailed')
    appStore.showError(message)
    console.error('Error creating TLS fingerprint collector session:', error)
  } finally {
    collectorSessionLoading.value = false
  }
}

const promoteCapture = async (record: TLSFingerprintCaptureRecord) => {
  if (!collectorSession.value || !record.promotion_eligible) return
  promotingCaptureId.value = record.id
  try {
    await adminAPI.tlsFingerprintProfiles.promoteCollectorCapture(
      collectorSession.value.token,
      record.id,
      { name: record.profile?.name }
    )
    appStore.showSuccess(t('admin.tlsFingerprintProfiles.collector.promoteSuccess'))
    await loadProfiles()
    await refreshCollectorCaptures()
  } catch (error: any) {
    appStore.showError(error?.response?.data?.detail || t('admin.tlsFingerprintProfiles.collector.promoteFailed'))
    console.error('Error promoting TLS fingerprint capture:', error)
  } finally {
    promotingCaptureId.value = null
  }
}

const approveProfile = async (profile: TLSFingerprintProfile) => {
  approvingProfileId.value = profile.id
  try {
    await adminAPI.tlsFingerprintProfiles.setApproval(profile.id, {
      approved_for_use: true,
      random_pool_eligible: false
    })
    appStore.showSuccess(t('admin.tlsFingerprintProfiles.approval.success'))
    await loadProfiles()
  } catch (error: any) {
    appStore.showError(error?.response?.data?.detail || t('admin.tlsFingerprintProfiles.approval.failed'))
  } finally {
    approvingProfileId.value = null
  }
}

const refreshCollectorCaptures = async () => {
  if (!collectorSession.value) return
  collectorCapturesLoading.value = true
  try {
    collectorCaptures.value = await adminAPI.tlsFingerprintProfiles.listCollectorCaptures(collectorSession.value.token)
  } catch (error: any) {
    console.error('Error loading TLS fingerprint captures:', error)
  } finally {
    collectorCapturesLoading.value = false
  }
}

const startCollectorPolling = () => {
  stopCollectorPolling()
  collectorPollTimer = window.setInterval(() => {
    refreshCollectorCaptures()
  }, 3000)
  refreshCollectorCaptures()
}

function stopCollectorPolling() {
  if (collectorPollTimer) {
    window.clearInterval(collectorPollTimer)
    collectorPollTimer = null
  }
}

const fillFormFromProfile = (profile: TLSFingerprintProfile) => {
  form.name = profile.name
  form.description = profile.description
  form.enable_grease = profile.enable_grease
  fieldInputs.cipher_suites = formatNumericArray(profile.cipher_suites)
  fieldInputs.curves = formatPlainNumericArray(profile.curves)
  fieldInputs.point_formats = formatPlainNumericArray(profile.point_formats)
  fieldInputs.signature_algorithms = formatNumericArray(profile.signature_algorithms)
  fieldInputs.signature_algorithms_cert = formatNumericArray(profile.signature_algorithms_cert)
  fieldInputs.alpn_protocols = (profile.alpn_protocols ?? []).join(', ')
  fieldInputs.supported_versions = formatNumericArray(profile.supported_versions)
  fieldInputs.key_share_groups = formatPlainNumericArray(profile.key_share_groups)
  fieldInputs.psk_modes = formatPlainNumericArray(profile.psk_modes)
  fieldInputs.extensions = formatNumericArray(profile.extensions)
}

const copyText = async (text: string) => {
  if (!text) return
  try {
    await navigator.clipboard.writeText(text)
    appStore.showSuccess(t('admin.tlsFingerprintProfiles.collector.copied'))
  } catch (error) {
    appStore.showError(t('admin.tlsFingerprintProfiles.collector.copyFailed'))
    console.error('Error copying text:', error)
  }
}

const downloadCA = () => {
  if (!activeCAPEM.value) return
  const blob = new Blob([activeCAPEM.value], { type: 'application/x-pem-file' })
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = 'tokenrouter-tls-collector-ca.pem'
  document.body.appendChild(link)
  link.click()
  document.body.removeChild(link)
  URL.revokeObjectURL(url)
}

const formatDateTime = (value?: string) => {
  if (!value) return '—'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? '—' : date.toLocaleString()
}

const formatCount = (value?: number) => new Intl.NumberFormat().format(
  Number.isFinite(value) ? Math.max(0, Number(value)) : 0
)

const formatClientKind = (kind: string) => {
  if (kind === 'claude_code') return 'Claude Code'
  if (kind === 'codex') return 'Codex CLI'
  if (kind === 'node') return 'Node.js'
  return t('admin.tlsFingerprintProfiles.collector.unknownClient')
}

const formatValidationStatus = (status?: string) => {
  switch (status) {
    case 'verified': return t('admin.tlsFingerprintProfiles.validation.verified')
    case 'rejected': return t('admin.tlsFingerprintProfiles.validation.rejected')
    case 'stale': return t('admin.tlsFingerprintProfiles.validation.stale')
    default: return t('admin.tlsFingerprintProfiles.validation.draft')
  }
}

const formatVerificationSource = (source?: string) => {
  switch (source) {
    case 'authenticated_ingress': return t('admin.tlsFingerprintProfiles.verificationSource.authenticatedIngress')
    case 'controlled_connect': return t('admin.tlsFingerprintProfiles.verificationSource.controlledConnect')
    case 'manual': return t('admin.tlsFingerprintProfiles.verificationSource.manual')
    default: return t('admin.tlsFingerprintProfiles.verificationSource.legacy')
  }
}

const validationStatusClass = (status?: string) => {
  switch (status) {
    case 'verified': return 'bg-green-100 text-green-700 dark:bg-green-900/40 dark:text-green-300'
    case 'rejected': return 'bg-red-100 text-red-700 dark:bg-red-900/40 dark:text-red-300'
    case 'stale': return 'bg-amber-100 text-amber-800 dark:bg-amber-900/40 dark:text-amber-200'
    default: return 'bg-gray-200 text-gray-600 dark:bg-dark-600 dark:text-gray-300'
  }
}

/**
 * 解析 tls-fingerprint-web 输出的 YAML 并填充表单。
 * 预期格式：
 *   # comment lines
 *   profile_key:
 *     name: "Profile Name"
 *     enable_grease: false
 *     cipher_suites: [4866, 4867, ...]
 *     alpn_protocols: ["h2", "http/1.1"]
 *     ...
 */
const parseYamlInput = () => {
  const text = yamlInput.value.trim()
  if (!text) return

  // 简单解析扁平 key-value 结构，支持数字数组和字符串数组。
  const lines = text.split('\n')

  let foundName = false

  for (const line of lines) {
    const trimmed = line.trim()
    // 跳过注释和空行。
    if (!trimmed || trimmed.startsWith('#')) continue

    // 匹配 key: value 形式的属性行。
    const match = trimmed.match(/^(\w+):\s*(.+)$/)
    if (!match) continue

    const [, key, rawValue] = match
    const value = rawValue.trim()

    switch (key) {
      case 'name': {
        // 去掉外层引号。
        const unquoted = parseYamlScalar(value)
        if (unquoted) {
          form.name = unquoted
          foundName = true
        }
        break
      }
      case 'description':
        form.description = parseYamlScalar(value) || null
        break
      case 'enable_grease':
        form.enable_grease = value === 'true'
        break
      case 'cipher_suites':
      case 'curves':
      case 'point_formats':
      case 'signature_algorithms':
      case 'signature_algorithms_cert':
      case 'supported_versions':
      case 'key_share_groups':
      case 'psk_modes':
      case 'extensions': {
        // 解析 YAML 数字数组。
        const arrMatch = value.match(/^\[(.*)?\]$/)
        if (arrMatch) {
          const inner = arrMatch[1] || ''
          fieldInputs[key as keyof typeof fieldInputs] = inner
            .split(',')
            .map(s => s.trim())
            .filter(s => s.length > 0)
            .join(', ')
        }
        break
      }
      case 'alpn_protocols': {
        // 解析 YAML 字符串数组。
        const arrMatch = value.match(/^\[(.*)?\]$/)
        if (arrMatch) {
          const inner = arrMatch[1] || ''
          fieldInputs.alpn_protocols = inner
            .split(',')
            .map(s => s.trim().replace(/^["']|["']$/g, ''))
            .filter(s => s.length > 0)
            .join(', ')
        }
        break
      }
    }
  }

  if (foundName) {
    appStore.showSuccess(t('admin.tlsFingerprintProfiles.form.yamlParsed'))
  } else {
    appStore.showError(t('admin.tlsFingerprintProfiles.form.yamlParseFailed'))
  }
}

// 粘贴后等待 v-model 更新再自动解析。
const handleYamlPaste = () => {
  setTimeout(() => parseYamlInput(), 50)
}

const closeFormModal = () => {
  showCreateModal.value = false
  showEditModal.value = false
  editingProfile.value = null
  resetForm()
}

// 解析逗号分隔的数字，支持十六进制与十进制。
const parseNumericArray = (input: string): number[] => {
  if (!input.trim()) return []
  return input
    .split(',')
    .map(s => s.trim())
    .filter(s => s.length > 0)
    .map(s => s.startsWith('0x') || s.startsWith('0X') ? parseInt(s, 16) : parseInt(s, 10))
    .filter(n => !isNaN(n))
}

// 解析逗号分隔的字符串。
const parseStringArray = (input: string): string[] => {
  if (!input.trim()) return []
  return input
    .split(',')
    .map(s => s.trim())
    .filter(s => s.length > 0)
}

// 数字按 4 位十六进制展示，便于对照 TLS ID。
const formatHex = (n: number): string => '0x' + n.toString(16).padStart(4, '0')

// 格式化数字数组。
const formatNumericArray = (arr: number[] | null | undefined): string => (arr ?? []).map(formatHex).join(', ')

// point_formats 与 psk_modes 是 uint8 语义，直接显示十进制。
const formatPlainNumericArray = (arr: number[] | null | undefined): string => (arr ?? []).join(', ')

// 解析导出 YAML 中的简单字符串标量，兼容 JSON 风格双引号与 YAML 单引号。
const parseYamlScalar = (value: string): string => {
  const trimmed = value.trim()
  if (!trimmed || trimmed === 'null' || trimmed === '~') return ''

  if (trimmed.startsWith('"') && trimmed.endsWith('"')) {
    try {
      return JSON.parse(trimmed)
    } catch {
      return trimmed.replace(/^"|"$/g, '')
    }
  }

  if (trimmed.startsWith("'") && trimmed.endsWith("'")) {
    return trimmed.slice(1, -1).replace(/''/g, "'")
  }

  return trimmed
}

// 字符串用 JSON 引号输出，避免名称和描述里的特殊字符破坏 YAML。
const formatYamlString = (value: string): string => JSON.stringify(value)

const formatYamlNumericArray = (arr: number[] | null | undefined, formatter: (n: number) => string = formatHex): string => {
  return `[${(arr ?? []).map(formatter).join(', ')}]`
}

const formatYamlStringArray = (arr: string[] | null | undefined): string => {
  return `[${(arr ?? []).map(formatYamlString).join(', ')}]`
}

const formatDecimal = (n: number): string => String(n)

// 导出的 YAML 与“粘贴 YAML 配置”入口保持同一字段结构，方便复制后再导入。
const buildProfileYaml = (profile: TLSFingerprintProfile): string => {
  return [
    'tls_fingerprint_profile:',
    `  name: ${formatYamlString(profile.name)}`,
    `  description: ${formatYamlString(profile.description || '')}`,
    `  enable_grease: ${profile.enable_grease ? 'true' : 'false'}`,
    `  cipher_suites: ${formatYamlNumericArray(profile.cipher_suites)}`,
    `  curves: ${formatYamlNumericArray(profile.curves, formatDecimal)}`,
    `  point_formats: ${formatYamlNumericArray(profile.point_formats, formatDecimal)}`,
    `  signature_algorithms: ${formatYamlNumericArray(profile.signature_algorithms)}`,
    `  signature_algorithms_cert: ${formatYamlNumericArray(profile.signature_algorithms_cert)}`,
    `  alpn_protocols: ${formatYamlStringArray(profile.alpn_protocols)}`,
    `  supported_versions: ${formatYamlNumericArray(profile.supported_versions)}`,
    `  key_share_groups: ${formatYamlNumericArray(profile.key_share_groups, formatDecimal)}`,
    `  psk_modes: ${formatYamlNumericArray(profile.psk_modes, formatDecimal)}`,
    `  extensions: ${formatYamlNumericArray(profile.extensions)}`
  ].join('\n')
}

const handleCopyYaml = (profile: TLSFingerprintProfile) => {
  copyText(buildProfileYaml(profile))
}

const handleEdit = (profile: TLSFingerprintProfile) => {
  editingProfile.value = profile
  fillFormFromProfile(profile)
  showEditModal.value = true
}

const handleDelete = (profile: TLSFingerprintProfile) => {
  deletingProfile.value = profile
  showDeleteDialog.value = true
}

const handleSubmit = async () => {
  if (!form.name.trim()) {
    appStore.showError(t('admin.tlsFingerprintProfiles.form.name') + ' ' + t('common.required'))
    return
  }

  submitting.value = true
  try {
    const data = {
      name: form.name.trim(),
      description: form.description?.trim() || null,
      enable_grease: form.enable_grease,
      cipher_suites: parseNumericArray(fieldInputs.cipher_suites),
      curves: parseNumericArray(fieldInputs.curves),
      point_formats: parseNumericArray(fieldInputs.point_formats),
      signature_algorithms: parseNumericArray(fieldInputs.signature_algorithms),
      signature_algorithms_cert: parseNumericArray(fieldInputs.signature_algorithms_cert),
      alpn_protocols: parseStringArray(fieldInputs.alpn_protocols),
      supported_versions: parseNumericArray(fieldInputs.supported_versions),
      key_share_groups: parseNumericArray(fieldInputs.key_share_groups),
      psk_modes: parseNumericArray(fieldInputs.psk_modes),
      extensions: parseNumericArray(fieldInputs.extensions)
    }

    if (showEditModal.value && editingProfile.value) {
      await adminAPI.tlsFingerprintProfiles.update(editingProfile.value.id, data)
      appStore.showSuccess(t('admin.tlsFingerprintProfiles.updateSuccess'))
    } else {
      await adminAPI.tlsFingerprintProfiles.create(data)
      appStore.showSuccess(t('admin.tlsFingerprintProfiles.createSuccess'))
    }

    closeFormModal()
    loadProfiles()
  } catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.tlsFingerprintProfiles.saveFailed'))
    console.error('Error saving TLS fingerprint profile:', error)
  } finally {
    submitting.value = false
  }
}

const confirmDelete = async () => {
  if (!deletingProfile.value) return

  try {
    await adminAPI.tlsFingerprintProfiles.delete(deletingProfile.value.id)
    appStore.showSuccess(t('admin.tlsFingerprintProfiles.deleteSuccess'))
    showDeleteDialog.value = false
    deletingProfile.value = null
    loadProfiles()
  } catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.tlsFingerprintProfiles.deleteFailed'))
    console.error('Error deleting TLS fingerprint profile:', error)
  }
}
</script>
