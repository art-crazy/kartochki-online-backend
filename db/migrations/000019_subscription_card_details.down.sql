alter table subscriptions
    drop column if exists payment_method_type,
    drop column if exists payment_method_last4,
    drop column if exists payment_method_card_type,
    drop column if exists payment_method_expiry_month,
    drop column if exists payment_method_expiry_year;
