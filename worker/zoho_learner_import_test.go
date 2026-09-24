package worker

import (
	"encoding/json"
	"testing"
)

func TestDecodeLearnersAcceptsEveryEnvelope(t *testing.T) {
	for _, input := range []string{
		`{"email":"a@x.com","superleapId":"Z1"}`,
		`[{"email":"a@x.com","superleapId":"Z1"}]`,
		`{"data":[{"email":"a@x.com","superleapId":"Z1"}]}`,
		`{"code":3000,"result":[{"email":"a@x.com","superleapId":"Z1"}]}`,
		`{"code":3000,"result":{"data":[{"email":"a@x.com","superleapId":"Z1"}]}}`,
	} {
		learners, err := decodeLearners(json.RawMessage(input))
		if err != nil || len(learners) != 1 || learners[0].SuperleapID != "Z1" {
			t.Fatalf("decodeLearners(%s) = %#v, %v", input, learners, err)
		}
	}
}

// Zoho sends blank amounts as "" and some text fields as numbers; those learners used to be
// dropped, so only the few perfectly typed ones were imported.
func TestDecodeLearnersKeepsLooselyTypedLearners(t *testing.T) {
	input := `{"result":[
		{"email":"a@x.com","balaceAmount":"","totalPaid":"5000","courseFee":20000,"phone":9876543210},
		{"email":"b@x.com","balaceAmount":null,"status":{},"financialDetails":[{"recordId":123634000053113330,"amount":"","verified":"No"}]},
		{"email":"c@x.com","totalPaid":"1,200","partialReminders":[{"recordId":"77","amount":1500.5}]},
		{"superleapId":"no-email"}
	]}`
	learners, err := decodeLearners(json.RawMessage(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(learners) != 3 {
		t.Fatalf("got %d learners, want 3: %#v", len(learners), learners)
	}
	if learners[0].Phone != "9876543210" || numberString(learners[0].CourseFee) != "20000" || learners[0].BalanceAmount != "" {
		t.Fatalf("learner a = %#v", learners[0])
	}
	if learners[1].Status != "" || numberString(learners[1].FinancialDetails[0].RecordID) != "123634000053113330" {
		t.Fatalf("learner b = %#v", learners[1])
	}
	if learners[2].TotalPaid != "1,200" || learners[2].PartialReminders[0].Amount != "1500.5" {
		t.Fatalf("learner c = %#v", learners[2])
	}
}

func TestNumberString(t *testing.T) {
	if got := numberString(zohoValue("123634000053113330")); got != "123634000053113330" {
		t.Fatalf("numberString() = %q", got)
	}
}
