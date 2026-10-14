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
import type {
  AutostartStatus,
  Preferences,
} from '@bindings/github.com/dongwlin/dan-tick/internal/desktop/models'

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
    // Optimistic: the switch shows the requested value while the platform write
    // and its verification round trip run.
    onMutate: (enabled) => {
      client.setQueryData<AutostartStatus | undefined>(applicationKeys.autostart, (current) =>
        current ? { ...current, enabled, needsRepair: false } : current,
      )
    },
    onSuccess: (status) => client.setQueryData(applicationKeys.autostart, status),
    // A failed confirmation can follow a successful system change. Read the
    // registered state again so the requested value cannot survive unverified.
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
    onMutate: (value) => client.setQueryData(applicationKeys.startupVisibility, value),
    onSuccess: (_, value) => client.setQueryData(applicationKeys.startupVisibility, value),
    // The write can succeed before its response fails. Read the stored value
    // again so an unconfirmed value cannot remain on screen.
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
    // Optimistic: the select shows the picked zone while the write runs.
    onMutate: (value) => {
      client.setQueryData<Preferences | undefined>(applicationKeys.preferences, (current) =>
        current ? { ...current, defaultTimezone: value } : current,
      )
    },
    // Read the stored value again on success and on failure. A re-read that
    // lands while the write is in flight, or a write whose response fails after
    // it landed, must not leave a zone on screen that the backend never stored.
    onSuccess: () => client.invalidateQueries({ queryKey: applicationKeys.preferences }),
    onError: () => client.invalidateQueries({ queryKey: applicationKeys.preferences }),
  })
}

export function useSetSchedulingEnabled() {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (value: boolean) => SetSchedulingEnabled(value),
    // Optimistic: the switch shows the requested value until the re-read lands.
    onMutate: (value) => {
      client.setQueryData<Preferences | undefined>(applicationKeys.preferences, (current) =>
        current ? { ...current, schedulingEnabled: value } : current,
      )
    },
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
