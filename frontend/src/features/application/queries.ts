import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query'
import {
  GetBuildInfo,
  GetOverview,
  GetPreferences,
  SetDefaultTimezone,
  SetSchedulingEnabled,
} from '@bindings/github.com/dongwlin/dan-tick/internal/desktop/applicationservice'
import {
  ExportDiagnostics,
  GetDiagnosticsStatus,
  OpenDataDirectory,
  OpenLogDirectory,
} from '@bindings/github.com/dongwlin/dan-tick/internal/desktop/diagnosticsservice'

export const applicationKeys = {
  all: ['application'] as const,
  preferences: ['application', 'preferences'] as const,
  overview: ['application', 'overview'] as const,
  diagnostics: ['application', 'diagnostics'] as const,
  buildInfo: ['application', 'build-info'] as const,
}

export function useBuildInfo() {
  return useQuery({
    queryKey: applicationKeys.buildInfo,
    queryFn: GetBuildInfo,
    staleTime: Infinity,
  })
}

export function usePreferences() {
  return useQuery({ queryKey: applicationKeys.preferences, queryFn: GetPreferences })
}

export function useOverview() {
  return useQuery({ queryKey: applicationKeys.overview, queryFn: GetOverview })
}

export function useSetDefaultTimezone() {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (value: string) => SetDefaultTimezone(value),
    onSettled: () => client.invalidateQueries({ queryKey: applicationKeys.all }),
  })
}

export function useSetSchedulingEnabled() {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (value: boolean) => SetSchedulingEnabled(value),
    onSettled: () => client.invalidateQueries({ queryKey: applicationKeys.all }),
  })
}

export function useDiagnosticsStatus() {
  return useQuery({ queryKey: applicationKeys.diagnostics, queryFn: GetDiagnosticsStatus })
}

export function useOpenLogDirectory() {
  return useMutation({ mutationFn: OpenLogDirectory })
}

export function useOpenDataDirectory() {
  return useMutation({ mutationFn: OpenDataDirectory })
}

export function useExportDiagnostics() {
  return useMutation({ mutationFn: ExportDiagnostics })
}
