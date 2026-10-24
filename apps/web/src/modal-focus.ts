export type FocusReturnCandidate<T>={target:T;connected:boolean;available:boolean};
// A dialog can outlive the button that opened it after a save, deletion or a
// project refresh. Prefer the original control, then the current main region.
export function modalFocusTarget<T>(opener:FocusReturnCandidate<T>|undefined,fallback:FocusReturnCandidate<T>|undefined):T|undefined{for(const candidate of [opener,fallback])if(candidate?.connected&&candidate.available)return candidate.target;}
