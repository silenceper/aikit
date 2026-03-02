<script setup lang="ts">
defineProps<{
  visible: boolean
  title: string
  message: string
}>()

const emit = defineEmits<{
  confirm: []
  cancel: []
}>()
</script>

<template>
  <Transition name="modal">
    <div
      v-if="visible"
      class="fixed inset-0 z-50 flex items-center justify-center p-4"
      @mousedown.self="emit('cancel')"
    >
      <div class="fixed inset-0 bg-black/40" aria-hidden="true" />
      <div
        class="modal-panel relative bg-white rounded-xl shadow-xl w-full max-w-sm"
        role="alertdialog"
        aria-modal="true"
      >
        <div class="px-6 pt-5 pb-2">
          <h2 class="text-lg font-semibold text-slate-900">{{ title }}</h2>
          <p class="text-sm text-slate-600 mt-2">{{ message }}</p>
        </div>
        <div class="flex justify-end gap-3 px-6 pb-5 pt-3">
          <button
            class="px-4 py-2 text-sm font-medium text-slate-700 bg-slate-100 hover:bg-slate-200
                   rounded-lg cursor-pointer focus:outline-none focus:ring-2 focus:ring-blue-500
                   transition-colors duration-200"
            @click="emit('cancel')"
          >
            Cancel
          </button>
          <button
            class="px-4 py-2 text-sm font-medium text-white bg-red-500 hover:bg-red-600
                   rounded-lg cursor-pointer focus:outline-none focus:ring-2 focus:ring-red-500
                   focus:ring-offset-2 transition-colors duration-200"
            @click="emit('confirm')"
          >
            Delete
          </button>
        </div>
      </div>
    </div>
  </Transition>
</template>
