package main

// type FpInfo struct {
// 	Description stktypes.Description `json:"description"`
// 	BtcPk       string               `json:"btc_pk"`
// 	Commision   sdkmath.LegacyDec    `json:"commission"`
// }
//
// func FuzzSignAndVerifySchnorrSig(f *testing.F) {
// 	testutil.AddRandomSeedsToFuzzer(f, 10)
// 	f.Fuzz(func(t *testing.T, seed int64) {
// 		r := rand.New(rand.NewSource(seed))
//
// 		tempDir := t.TempDir()
// 		homeDir := filepath.Join(tempDir, "eots-home")
// 		rootCmd := NewRootCmd()
//
// 		// init config in home folder
// 		hFlag := fmt.Sprintf("--home=%s", homeDir)
// 		err := rootCmd.ex.Run([]string{"eotsd", "init", hFlag})
// 		require.NoError(t, err)
//
// 		keyName := testutil.GenRandomHexStr(r, 10)
// 		keyNameFlag := fmt.Sprintf("--key-name=%s", keyName)
//
// 		outputKeysAdd := appRunWithOutput(r, t, app, []string{"eotsd", "keys", "add", hFlag, keyNameFlag})
// 		keyOutJson := searchInTxt(outputKeysAdd, "for recovery):")
//
// 		var keyOut KeyOutput
// 		err = json.Unmarshal([]byte(keyOutJson), &keyOut)
// 		require.NoError(t, err)
//
// 		fpInfoPath := filepath.Join(tempDir, "fpInfo.json")
// 		writeFpInfoToFile(r, t, fpInfoPath, keyOut.PubKeyHex)
//
// 		eotsBtcPkFlag := fmt.Sprintf("--eots-pk=%s", keyOut.PubKeyHex)
// 		dataSignedBtcPk := appRunSignSchnorr(r, t, app, []string{fpInfoPath, hFlag, eotsBtcPkFlag})
// 		err = app.Run([]string{"eotsd", "verify-schnorr-sig", fpInfoPath, eotsBtcPkFlag, fmt.Sprintf("--signature=%s", dataSignedBtcPk.SchnorrSignatureHex)})
// 		require.NoError(t, err)
//
// 		dataSignedKeyName := appRunSignSchnorr(r, t, app, []string{fpInfoPath, hFlag, keyNameFlag})
// 		err = app.Run([]string{"eotsd", "verify-schnorr-sig", fpInfoPath, eotsBtcPkFlag, fmt.Sprintf("--signature=%s", dataSignedKeyName.SchnorrSignatureHex)})
// 		require.NoError(t, err)
//
// 		// check if both generated signatures match
// 		require.Equal(t, dataSignedBtcPk.PubKeyHex, dataSignedKeyName.PubKeyHex)
// 		require.Equal(t, dataSignedBtcPk.SchnorrSignatureHex, dataSignedKeyName.SchnorrSignatureHex)
// 		require.Equal(t, dataSignedBtcPk.SignedDataHashHex, dataSignedKeyName.SignedDataHashHex)
//
// 		// sign with both keys and eots-pk, should give eots-pk preference
// 		dataSignedBoth := appRunSignSchnorr(r, t, app, []string{fpInfoPath, hFlag, eotsBtcPkFlag, keyNameFlag})
// 		require.Equal(t, dataSignedBoth, dataSignedKeyName)
//
// 		// the keyname can even be from a invalid keyname, since it gives eots-pk preference
// 		badKeyname := "badKeyName"
// 		dataSignedBothBadKeyName := appRunSignSchnorr(r, t, app, []string{fpInfoPath, hFlag, eotsBtcPkFlag, fmt.Sprintf("--key-name=%s", badKeyname)})
// 		require.Equal(t, badKeyname, dataSignedBothBadKeyName.KeyName)
// 		require.Equal(t, dataSignedBtcPk.PubKeyHex, dataSignedBothBadKeyName.PubKeyHex)
// 		require.Equal(t, dataSignedBtcPk.SchnorrSignatureHex, dataSignedBothBadKeyName.SchnorrSignatureHex)
// 		require.Equal(t, dataSignedBtcPk.SignedDataHashHex, dataSignedBothBadKeyName.SignedDataHashHex)
// 	})
// }
//
// func appRunSignSchnorr(r *rand.Rand, t *testing.T, app *cli.App, arguments []string) main.DataSigned {
// 	args := []string{"eotsd", "sign-schnorr"}
// 	args = append(args, arguments...)
// 	outputSign := appRunWithOutput(r, t, app, args)
// 	signatureStr := searchInTxt(outputSign, "")
//
// 	var dataSigned main.DataSigned
// 	err := json.Unmarshal([]byte(signatureStr), &dataSigned)
// 	require.NoError(t, err)
//
// 	return dataSigned
// }
//
// func appRunWithOutput(r *rand.Rand, t *testing.T, app *cli.App, arguments []string) (output string) {
// 	outPut := filepath.Join(t.TempDir(), fmt.Sprintf("%s-out.txt", testutil.GenRandomHexStr(r, 10)))
// 	outPutFile, err := os.Create(outPut)
// 	require.NoError(t, err)
// 	defer outPutFile.Close()
//
// 	// set file to stdout to read.
// 	oldStd := os.Stdout
// 	os.Stdout = outPutFile
//
// 	err = app.Run(arguments)
// 	require.NoError(t, err)
//
// 	// set to old stdout
// 	os.Stdout = oldStd
// 	return readFromFile(t, outPutFile)
// }
//
// func searchInTxt(text, search string) string {
// 	idxOfRecovery := strings.Index(text, search)
// 	jsonKeyOutputOut := text[idxOfRecovery+len(search):]
// 	return strings.ReplaceAll(jsonKeyOutputOut, "\n", "")
// }
//
// func readFromFile(t *testing.T, f *os.File) string {
// 	_, err := f.Seek(0, 0)
// 	require.NoError(t, err)
//
// 	buf := new(bytes.Buffer)
// 	_, err = buf.ReadFrom(f)
// 	require.NoError(t, err)
// 	return buf.String()
// }
//
// func writeFpInfoToFile(r *rand.Rand, t *testing.T, fpInfoPath, btcPk string) {
// 	desc := testutil.RandomDescription(r)
// 	fpInfo := FpInfo{
// 		BtcPk:       btcPk,
// 		Commision:   sdkmath.LegacyMustNewDecFromStr("0.5"),
// 		Description: *desc,
// 	}
//
// 	bzFpInfo, err := json.Marshal(fpInfo)
// 	require.NoError(t, err)
//
// 	fpInfoFile, err := os.Create(fpInfoPath)
// 	require.NoError(t, err)
//
// 	_, err = fpInfoFile.Write(bzFpInfo)
// 	require.NoError(t, err)
// 	fpInfoFile.Close()
// }
