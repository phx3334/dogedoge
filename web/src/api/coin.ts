import request from './request'
import type { CoinResultResp, CoinLedgerItem, PaginatedResp } from '@/types'

// 视频投币
export function coinVideo(video_id: number, amount: 1 | 2) {
  return request.post<any, CoinResultResp>('/coin/video', { video_id, amount })
}

// 硬币流水
export function getCoinLedger(reason_type: string = '', page: number = 1, page_size: number = 20) {
  return request.get<any, PaginatedResp<CoinLedgerItem>>('/coin/ledger', {
    params: { reason_type, page, page_size },
  })
}
