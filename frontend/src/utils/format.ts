import type {ScheduleExpiryReason} from '../types/entities';

export const formatPct=(value:number)=>`${value.toFixed(1)}%`; export const formatDate=(value:string)=>new Intl.DateTimeFormat('zh-CN',{month:'short',day:'numeric',hour:'2-digit',minute:'2-digit'}).format(new Date(value));

export const EXPIRY_REASON_LABELS: Record<ScheduleExpiryReason, string> = {
  reading_backfilled: '读数补录：纳入新增含水率读数后旧输入失效',
  reading_voided: '读数作废：有效样本减少，旧计划输入失效',
  reading_corrected: '读数修正：替换读数后旧计划输入失效',
};

export const expiryReasonLabel = (reason?: string): string =>
  reason ? (EXPIRY_REASON_LABELS[reason as ScheduleExpiryReason] ?? reason) : '';
