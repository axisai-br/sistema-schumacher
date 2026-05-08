import type { ColumnConfig, FormFieldConfig } from "../../components/layout/CRUDListPage";

type CrudPageConfig<TItem, TForm> = {
  formFields: FormFieldConfig<TForm>[];
  columns: ColumnConfig<TItem>[];
  initialForm: TForm;
  mapItemToForm: (item: TItem) => TForm;
  searchFilter: (item: TItem, term: string) => boolean;
};

export function createCrudPageConfig<TItem, TForm>(config: CrudPageConfig<TItem, TForm>) {
  return config;
}
