resource "openlineage_job" "example" {
  namespace   = "airflow"
  name        = "analytics.aggregate_product_sales"
  description = "Joins products and order items to produce sales statistics"

  inputs {
    namespace = "bigquery"
    name      = "my-project.raw.products"
  }

  inputs {
    namespace = "bigquery"
    name      = "my-project.raw.order_items"
  }

  outputs {
    namespace = "bigquery"
    name      = "my-project.analytics.product_sales"

    column_lineage {
      fields {
        name = "total_price"

        input_field {
          namespace = "bigquery"
          name      = "my-project.raw.order_items"
          field     = "unit_price"

          transformation {
            type        = "DIRECT"
            subtype     = "IDENTITY"
            description = "Sum of unit prices"
            masking     = false
          }
        }
      }
    }
  }
}

