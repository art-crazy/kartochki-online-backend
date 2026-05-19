alter table subscriptions
    add column payment_method_type         varchar,
    add column payment_method_last4        varchar,
    add column payment_method_card_type    varchar,
    add column payment_method_expiry_month varchar,
    add column payment_method_expiry_year  varchar;

comment on column subscriptions.payment_method_type         is 'Тип сохранённого способа оплаты, например "bank_card".';
comment on column subscriptions.payment_method_last4        is 'Последние 4 цифры карты. NULL, если карта не привязана.';
comment on column subscriptions.payment_method_card_type    is 'Платёжная система карты: Visa, MasterCard и т.д.';
comment on column subscriptions.payment_method_expiry_month is 'Месяц истечения срока карты (двузначный, "01"–"12").';
comment on column subscriptions.payment_method_expiry_year  is 'Год истечения срока карты (четырёхзначный, например "2027").';
