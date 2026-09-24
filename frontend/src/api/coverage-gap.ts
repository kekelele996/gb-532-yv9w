import { apiClient } from './client'
import type { DetectCoverage, CoverageGap, CoverageResult, GapClaimFilter } from '../types/coverage-gap'
import type { GapState } from '../types/enums/gap-severity'
export const coverageGapApi={
  list:(claim:GapClaimFilter='all')=>apiClient.page<CoverageGap[]>(`/coverage-gaps?page_size=100&claim=${claim}`),
  get:(id:number)=>apiClient.get<CoverageGap>(`/coverage-gaps/${id}`),
  detect:(body:DetectCoverage,key:string)=>apiClient.post<CoverageResult>('/coverage-gaps/detect',body,{'Idempotency-Key':key}),
  claim:(id:number,expectedVersion:number,claimNote:string)=>apiClient.post<CoverageGap>(`/coverage-gaps/${id}/claim`,{expected_version:expectedVersion,claim_note:claimNote}),
  release:(id:number,expectedVersion:number,reason:string)=>apiClient.post<CoverageGap>(`/coverage-gaps/${id}/release`,{expected_version:expectedVersion,reason}),
  transition:(id:number,target:GapState,version:number,note:string)=>apiClient.post<CoverageGap>(`/coverage-gaps/${id}/transition`,{target_state:target,expected_version:version,review_note:note})}
