import { useQuery } from '@tanstack/react-query'
import { boardsApi } from '../api/projects'
import type { BoardTypeDef } from '../api/types'

// Board types (incl. icon, display name and presentation spec) are runtime data
// from the Board Registry. Cached briefly so newly registered types show up without
// a reload. Shared by the create-board dialog and the board renderer.
export function useBoardTypes() {
  return useQuery({
    queryKey: ['board-types'],
    queryFn: () => boardsApi.listBoardTypes(),
    staleTime: 60_000,
  })
}

// Resolves a single board-type definition by slug from the cached list.
export function useBoardType(type: string | undefined): BoardTypeDef | undefined {
  const { data } = useBoardTypes()
  if (!type) return undefined
  return data?.data.find((t) => t.type === type)
}
