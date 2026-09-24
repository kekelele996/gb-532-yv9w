import { create } from 'zustand'
import { coverageGapApi } from '../api/coverage-gap'
import type { CoverageEvidence, CoverageGap, DetectCoverage, GapClaimFilter } from '../types/coverage-gap'
import type { GapState } from '../types/enums/gap-severity'

interface GapStore{items:CoverageGap[];selected:CoverageGap|null;evidence:CoverageEvidence|null;loading:boolean;claimFilter:GapClaimFilter;fetch:()=>Promise<void>;setClaimFilter:(filter:GapClaimFilter)=>void;select:(item:CoverageGap)=>void;detect:(body:DetectCoverage)=>Promise<void>;claim:(item:CoverageGap,note:string)=>Promise<void>;release:(item:CoverageGap,reason:string)=>Promise<void>;transition:(item:CoverageGap,target:GapState,note:string)=>Promise<void>}

function patchItem(items:CoverageGap[],updated:CoverageGap):CoverageGap[]{return items.map(value=>value.id===updated.id?updated:value)}
export const useCoverageGapStore=create<GapStore>((set,get)=>({items:[],selected:null,evidence:null,loading:false,claimFilter:'all',
  fetch:async()=>{set({loading:true});try{const filter=get().claimFilter;const response=await coverageGapApi.list(filter);set(state=>({items:response.data,selected:state.selected?response.data.find(value=>value.id===state.selected?.id)??state.selected:response.data[0]??null}))}finally{set({loading:false})}},
  setClaimFilter:(claimFilter)=>{set({claimFilter,selected:null});void get().fetch()},
  select:(selected)=>set({selected,evidence:null}),
  detect:async(body)=>{const key=`coverage-${body.survey_area_id}-${body.source_run_ids.join('-')}-${body.algorithm_version}-${body.resolution_m}`;const response=await coverageGapApi.detect(body,key);set(state=>({items:state.items.some(value=>value.id===response.data.gap.id)?state.items:[response.data.gap,...state.items],selected:response.data.gap,evidence:response.data.evidence}));},
  claim:async(item,note)=>{const updated=(await coverageGapApi.claim(item.id,item.version,note)).data;set(state=>({items:patchItem(state.items,updated),selected:updated}))},
  release:async(item,reason)=>{const updated=(await coverageGapApi.release(item.id,item.version,reason)).data;set(state=>({items:patchItem(state.items,updated),selected:updated}))},
  transition:async(item,target,note)=>{const updated=(await coverageGapApi.transition(item.id,target,item.version,note)).data;set(state=>({items:patchItem(state.items,updated),selected:updated}))}
}))
