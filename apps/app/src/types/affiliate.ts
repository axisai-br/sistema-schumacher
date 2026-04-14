export type CurrentUser = {
  user_id: string;
  roles: string[];
  can_access_saldo: boolean;
  has_recipient: boolean;
};

export type AffiliateBalance = {
  available_amount: number;
  waiting_funds_amount: number;
  transferred_amount: number;
  currency: string;
  can_withdraw: boolean;
  withdraw_block_reason?: string;
  anticipation_info?: {
    enabled: boolean;
    type?: string;
    delay?: number;
    volume_percentage?: number;
  };
};

export type AffiliateWithdrawalItem = {
  id: string;
  amount: number;
  currency: string;
  status: string;
  transfer_id?: string;
  requested_at: string;
  processed_at?: string;
};

export type AffiliateWithdrawalsHistory = {
  items: AffiliateWithdrawalItem[];
  pagination: {
    limit: number;
    offset: number;
    total_estimate: number;
  };
};

export type AffiliateWithdrawResponse = {
  withdrawal_id: string;
  transfer_id?: string;
  status: "success" | "pending" | "failed";
  message: string;
  requested_amount: number;
  currency: string;
};
