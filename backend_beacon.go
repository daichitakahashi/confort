package confort

/*
TODO:
  - CFT_BEACON_ENDPOINT 環境変数を使用してエンドポイントをセットする
  - namespaceはgRPCで問い合わせる
  - cmd/confort でbeaconを起動させ、標準出力にエンドポイントを出力
    - export CFT_BEACON=`go run github.com/daichitakahashi/confort/cmd/confort -p 9999 -namespace hogehoge*`
  - New には WithBeacon を用意すれば良いとして、UniqueのBeacon登録はどうやったら良いか
    - WithGlobalUniqueness(beaconStore string) で CFT_BEACON_ENDPOINT があれば登録する
*/
