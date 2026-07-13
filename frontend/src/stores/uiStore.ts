import { create } from 'zustand'

interface UIState {
  selectedTaskId: string | null
  selectTask: (id: string | null) => void
  createProjectModal: boolean
  openCreateProject: () => void
  closeCreateProject: () => void
}

export const useUIStore = create<UIState>((set) => ({
  selectedTaskId: null,
  selectTask: (id) => set({ selectedTaskId: id }),
  createProjectModal: false,
  openCreateProject: () => set({ createProjectModal: true }),
  closeCreateProject: () => set({ createProjectModal: false }),
}))
