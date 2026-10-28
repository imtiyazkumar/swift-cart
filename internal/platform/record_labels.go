package platform

import (
	"sort"
	"strings"
)

// recordLabelSQL never includes passwords, token hashes or bank credentials.
// It is used only by the administrator projection, not public/partner responses.
func recordLabelSQL(table, a string) string {
	c := func(name string) string { return a + "." + name }
	text := func(value string) string { return "COALESCE(NULLIF(" + value + ",''),'Not named')" }
	person := func(field string) string {
		return "(SELECT COALESCE(NULLIF(label_user.full_name,''),NULLIF(label_user.email,''),'User') FROM users label_user WHERE label_user.id=" + c(field) + ")"
	}
	order := func() string {
		return "COALESCE((SELECT order_number FROM orders label_order WHERE label_order.id=" + c("order_id") + "),'Order')"
	}
	switch table {
	case "users":
		return "COALESCE(NULLIF(" + c("full_name") + ",''),NULLIF(" + c("email") + ",''),'Unnamed user')"
	case "restaurants", "stations", "menu_categories", "menu_items":
		return text(c("name"))
	case "item_variants":
		return "COALESCE((SELECT name FROM menu_items label_item WHERE label_item.id=" + c("menu_item_id") + "),'Menu item')||' / '||" + text(c("name"))
	case "riders":
		return "COALESCE(" + person("user_id") + ",'Delivery agent')"
	case "orders":
		return text(c("order_number"))
	case "user_addresses":
		return "concat_ws(' / ',NULLIF(" + c("label") + ",''),NULLIF(" + c("address_line1") + ",''),NULLIF(" + c("city") + ",''))"
	case "coupons":
		return text(c("code"))
	case "carts":
		return "'Cart / '||COALESCE(" + person("user_id") + ",'Customer')"
	case "order_items", "cart_items":
		return "COALESCE((SELECT name FROM menu_items label_item WHERE label_item.id=" + c("menu_item_id") + "),'Menu item')||' x '||" + c("quantity") + "::text"
	case "restaurant_documents", "rider_documents":
		return "replace(" + text(c("document_type")) + ",'_',' ')"
	case "restaurant_bank_accounts", "delivery_agent_bank_details":
		return "concat_ws(' / ',NULLIF(" + c("account_holder") + ",''),NULLIF(" + c("bank_name") + ",''))"
	case "payments":
		return "'Payment / '||COALESCE((SELECT order_number FROM orders label_order WHERE label_order.id=" + c("order_id") + "),NULLIF(" + c("provider_payment_id") + ",''),'Registration')"
	case "payment_intents":
		return "replace(" + text(c("purpose")) + ",'_',' ')||' / '||COALESCE((SELECT order_number FROM orders label_order WHERE label_order.id=" + c("order_id") + ")," + c("stage") + ",'Payment')"
	case "restaurant_earnings", "delivery_agent_earnings":
		return "'Earning / '||" + order()
	case "restaurant_payouts", "delivery_agent_payouts":
		return "'Payout / '||COALESCE(NULLIF(" + c("provider_transfer_id") + ",'')," + c("method") + ",'Bank transfer')"
	case "refunds":
		return "'Refund / '||" + order()
	case "registration_fees":
		return "CASE WHEN " + c("restaurant_id") + " IS NOT NULL THEN 'Restaurant registration' ELSE 'Agent registration' END"
	case "system_activity_logs":
		return text(c("action"))
	case "refresh_tokens":
		return "'Session / '||COALESCE(NULLIF(" + c("device_label") + ",''),'Device')"
	case "otp_challenges":
		return "'Verification / '||COALESCE(" + c("purpose") + ",'Login')"
	case "delivery_assignments":
		return "'Delivery / '||" + order()
	case "rider_locations":
		return "'Location / '||COALESCE((SELECT COALESCE(u.full_name,u.email) FROM riders r JOIN users u ON u.id=r.user_id WHERE r.id=" + c("rider_id") + "),'Agent')"
	case "financial_transactions":
		return "replace(" + text(c("transaction_type")) + ",'_',' ')||' / '||replace(" + text(c("account_code")) + ",'_',' ')"
	case "reconciliation_runs":
		return "'Reconciliation / '||" + text(c("source")) + "||' / '||to_char(" + c("period_start") + ",'DD Mon YYYY')"
	}
	return "'" + strings.ReplaceAll(table, "_", " ") + "'"
}

func relationSQL(table, condition string) string {
	return "(SELECT jsonb_build_object('id',related_record.uuid,'label'," + recordLabelSQL(table, "related_record") + ",'resource','" + strings.ReplaceAll(table, "_", "-") + "') FROM " + table + " related_record WHERE " + condition + ")"
}

func adminProject(table, alias string) string {
	relations := []string{}
	for _, column := range tables[table].Columns {
		target := references[column.Name]
		if column.Name == "actor_id" {
			target = "users"
		}
		if target != "" {
			relations = append(relations, "'"+column.Name+"',"+relationSQL(target, "related_record.id="+alias+"."+column.Name))
		}
	}
	if table == "system_activity_logs" {
		names := make([]string, 0, len(tables))
		for name := range tables {
			names = append(names, name)
		}
		sort.Strings(names)
		cases := []string{}
		for _, name := range names {
			cases = append(cases, "WHEN '"+strings.ToUpper(name)+"' THEN "+relationSQL(name, "related_record.uuid="+alias+".entity_uuid"))
		}
		entity := "CASE upper(" + alias + ".entity_type) " + strings.Join(cases, " ") + " ELSE NULL END"
		for _, key := range []string{"entity_id", "entity_uuid", "target_id"} {
			relations = append(relations, "'"+key+"',"+entity)
		}
	}
	metadata := "'{}'::jsonb"
	if len(relations) > 0 {
		metadata = "jsonb_strip_nulls(jsonb_build_object(" + strings.Join(relations, ",") + "))"
	}
	return project(table, alias) + "||jsonb_build_object('_display_name'," + recordLabelSQL(table, alias) + ",'_relations'," + metadata + ")"
}
