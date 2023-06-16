package common

func VerifyWalletSign(wallet string, timestamp int64, sign string) error {
	//// check timestamp：如果 timestamp 比现在晚了超过多少秒就算超期
	//milliseconds := time.Now().UnixMilli() // unit: millisecond
	//if milliseconds-timestamp > 30*1000 {
	//	return errors.New("signature out of date")
	//}
	//
	//// verify sign
	//msg := fmt.Sprintf("rfa#%s#%d", wallet, timestamp) // `rfa#{Wallet}#{Timestamp}`
	//ok, err := ethsign.SignVerify(msg, sign, wallet)
	//if err != nil {
	//	return err
	//}
	//if !ok {
	//	return errors.New("your sign is not correct")
	//}

	return nil
}
