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
import {
  GetAutostartStatus,
  SetAutostartEnabled,
} from '@bindings/github.com/dongwlin/dan-tick/internal/desktop/autostartservice'
import {
  GetStartupVisibility,
  SetStartupVisibility,
} from '@bindings/github.com/dongwlin/dan-tick/internal/desktop/windowservice'

export const applicationKeys = {
  all: ['application'] as const,
  preferences: ['application', 'preferences'] as const,
  overview: ['application', 'overview'] as const,
  diagnostics: ['application', 'diagnostics'] as const,
  buildInfo: ['application', 'build-info'] as const,
  autostart: ['application', 'autostart'] as const,
  startupVisibility: ['application', 'startup-visibility'] as const,
}

export function useAutostartStatus() {
  return useQuery({ queryKey: applicationKeys.autostart, queryFn: GetAutostartStatus })
}

export function useSetAutostartEnabled() {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (enabled: boolean) => SetAutostartEnabled(enabled),
    onSuccess: (status) => client.setQueryData(applicationKeys.autostart, status),
    // A failed confirmation can follow a successful system change. Read the
    // registered state again before allowing another attempt.
    onError: () => client.invalidateQueries({ queryKey: applicationKeys.autostart }),
  })
}

export function useStartupVisibility() {
  return useQuery({ queryKey: applicationKeys.startupVisibility, queryFn: GetStartupVisibility })
}

export function useSetStartupVisibility() {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (value: string) => SetStartupVisibility(value),
    onSuccess: (_, value) => client.setQueryData(applicationKeys.startupVisibility, value),
    // The write can succeed before its response fails. Read the stored value
    // again so the switch never shows a state the backend does not hold.
    onError: () => client.invalidateQueries({ queryKey: applicationKeys.startupVisibility }),
  })
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
