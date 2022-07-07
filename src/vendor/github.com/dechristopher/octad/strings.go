package octad

import "fmt"

const methodName = "NoMethodCheckmateResignationDrawOfferStalemateThreefoldRepetitionFivefoldRepetitionFiftyMoveRuleSeventyFiveMoveRuleInsufficientMaterial"

var methodIndex = [...]uint8{0, 8, 17, 28, 37, 46, 65, 83, 96, 115, 135}

func (i Method) String() string {
	if i >= Method(len(methodIndex)-1) {
		return fmt.Sprintf("Method(%d)", i)
	}
	return methodName[methodIndex[i]:methodIndex[i+1]]
}
