
export type ModelsListCandidatesMode = "create" | "edit";
export type ModelsListCandidatesStatus = "idle" | "loading" | "ready" | "error";

export interface ModelsListCandidatesRequest {
  mode: ModelsListCandidatesMode;
  groupID: number;
}

export interface ModelsListCandidatesTracker {
  next(request: ModelsListCandidatesRequest): number;
  invalidate(mode: ModelsListCandidatesMode): void;
  isCurrent(requestID: number, request: ModelsListCandidatesRequest): boolean;
}

export const createModelsListCandidatesTracker = (): ModelsListCandidatesTracker => {
  let currentRequestID = 0;
  const currentByMode: Partial<Record<ModelsListCandidatesMode, {
    id: number;
    request: ModelsListCandidatesRequest;
  }>> = {};

  return {
    next(request) {
      currentRequestID += 1;
      currentByMode[request.mode] = {
        id: currentRequestID,
        request: { ...request },
      };
      return currentRequestID;
    },
    // 关闭表单后，响应序号随该表单失效。
    invalidate(mode) {
      delete currentByMode[mode];
    },
    isCurrent(requestID, request) {
      const current = currentByMode[request.mode];
      return (
        current?.id === requestID &&
        current.request.groupID === request.groupID
      );
    },
  };
};
