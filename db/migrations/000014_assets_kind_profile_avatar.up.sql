alter table assets
    drop constraint assets_kind_check;

alter table assets
    add constraint assets_kind_check check (kind in ('source_image', 'generated_card', 'archive', 'profile_avatar'));
