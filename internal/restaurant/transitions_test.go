package restaurant

import "testing"

func TestRestaurantCannotSkipOrReversePreparation(t *testing.T) {
    for _, pair := range [][2]string{{"CREATED", "READY_FOR_PICKUP"}, {"ACCEPTED", "DELIVERED"}, {"DELIVERED", "PREPARING"}, {"READY_FOR_PICKUP", "ACCEPTED"}, {"CREATED", "PREPARING"}} {
        if validRestaurantTransition(pair[0], pair[1]) {
            t.Fatalf("allowed %s -> %s", pair[0], pair[1])
        }
    }
    if !validRestaurantTransition("CREATED", "ACCEPTED") || !validRestaurantTransition("ACCEPTED", "PREPARING") || !validRestaurantTransition("PREPARING", "READY_FOR_PICKUP") {
        t.Fatal("valid fulfillment path rejected")
    }
}
