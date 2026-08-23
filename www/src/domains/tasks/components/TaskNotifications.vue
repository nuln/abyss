<template>
  <div v-if="activeTasks.length > 0" class="task-notifications">
    <div v-for="task in activeTasks" :key="task.id" class="task-item" :class="task.status">
      <div class="task-info">
        <span class="task-description">{{ task.message || task.description }}</span>
        <span class="task-progress-text">{{ Math.round(task.progress * 100) }}%</span>
      </div>
      <div class="progress-bar">
        <div class="progress-fill" :style="{ width: (task.progress * 100) + '%' }"></div>
      </div>
      <button v-if="isDismissible(task)" class="dismiss-btn" @click="taskStore.removeTask(task.id)">
        <i class="material-icons">close</i>
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useTaskStore } from '@/domains/tasks/store';

const taskStore = useTaskStore();

const activeTasks = computed(() => {
  return Object.values(taskStore.tasks).sort((a, b) => b.updatedAt - a.updatedAt);
});

function isDismissible(task: any) {
  return ['completed', 'failed', 'cancelled'].includes(task.status);
}
</script>

<style scoped>
.task-notifications {
  position: fixed;
  bottom: 2rem;
  right: 2rem;
  width: 320px;
  display: flex;
  flex-direction: column;
  gap: 1rem;
  z-index: 9999;
}

.task-item {
  background: var(--surface-card);
  border-radius: var(--radius-lg);
  padding: 1rem;
  box-shadow: var(--shadow-lg);
  border: 1px solid var(--border-subtle);
  position: relative;
  transition: all 0.3s ease;
  overflow: hidden;
}

.task-item.completed {
  border-color: var(--color-success, #4caf50);
}

.task-item.failed {
  border-color: var(--color-error, #f44336);
}

.task-info {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 0.5rem;
  gap: 0.5rem;
}

.task-description {
  font-size: 0.875rem;
  font-weight: 500;
  color: var(--text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.task-progress-text {
  font-size: 0.75rem;
  color: var(--text-secondary);
  font-variant-numeric: tabular-nums;
}

.progress-bar {
  height: 4px;
  background: var(--color-gray-100);
  border-radius: 2px;
  overflow: hidden;
}

.progress-fill {
  height: 100%;
  background: var(--color-accent);
  transition: width 0.3s ease;
}

.task-item.completed .progress-fill {
  background: var(--color-success, #4caf50);
}

.task-item.failed .progress-fill {
  background: var(--color-error, #f44336);
}

.dismiss-btn {
  position: absolute;
  top: 0.25rem;
  right: 0.25rem;
  background: none;
  border: none;
  color: var(--text-tertiary);
  cursor: pointer;
  padding: 0.25rem;
  border-radius: 50%;
  display: flex;
  opacity: 0;
  transition: opacity 0.2s;
}

.task-item:hover .dismiss-btn {
  opacity: 1;
}

.dismiss-btn:hover {
  background: var(--color-gray-100);
  color: var(--text-secondary);
}

.dismiss-btn i {
  font-size: 1rem;
}
</style>
