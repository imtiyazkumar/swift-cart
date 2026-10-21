CREATE OR REPLACE FUNCTION notifications.record_order_status() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO notifications.notification_logs(user_id,channel,template_key,payload,status)
    SELECT DISTINCT recipient,'IN_APP','order_status',jsonb_build_object(
        'title','Order status update','message','Order ' || left(NEW.order_id::text,8) || ': ' || NEW.status::text,
        'order_id',NEW.order_id,'order_status',NEW.status),'UNREAD'
    FROM (
        SELECT u.id AS recipient FROM public.orders o JOIN public.customers c ON c.id=o.customer_id
        JOIN public.users u ON u.id=COALESCE(c.user_id,c.id) WHERE o.id=NEW.order_id
        UNION
        SELECT m.owner_id FROM public.orders o JOIN public.merchants m ON m.id=o.merchant_id WHERE o.id=NEW.order_id
        UNION
        SELECT p.user_id FROM delivery.delivery_assignments a JOIN delivery.delivery_partners p ON p.id=a.partner_id WHERE a.order_id=NEW.order_id
    ) recipients WHERE recipient IS NOT NULL;
    RETURN NEW;
END;
$$;
DROP TRIGGER IF EXISTS record_order_status_notification ON orders.order_status_history;
CREATE TRIGGER record_order_status_notification AFTER INSERT ON orders.order_status_history
    FOR EACH ROW EXECUTE FUNCTION notifications.record_order_status();
